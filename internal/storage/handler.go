package storage

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/go-chi/chi/v5"
)

// Handler serves storage HTTP endpoints.
type Handler struct {
	svc         *Service
	logger      *slog.Logger
	maxFileSize int64
}

// NewHandler creates a new storage handler.
func NewHandler(svc *Service, logger *slog.Logger, maxFileSize int64) *Handler {
	return &Handler{
		svc:         svc,
		logger:      logger,
		maxFileSize: maxFileSize,
	}
}

// Routes returns a chi.Router with storage endpoints mounted.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{bucket}", h.handleList)
	r.Post("/{bucket}", h.handleUpload)
	r.Get("/{bucket}/*", h.handleServe)
	r.Delete("/{bucket}/*", h.handleDelete)
	r.Post("/{bucket}/{name}/sign", h.handleSign)
	return r
}

type listResponse struct {
	Items      []Object `json:"items"`
	TotalItems int      `json:"totalItems"`
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	prefix := r.URL.Query().Get("prefix")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	objects, total, err := h.svc.ListObjects(r.Context(), bucket, prefix, limit, offset)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("list error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if objects == nil {
		objects = []Object{}
	}
	httputil.WriteJSON(w, http.StatusOK, listResponse{Items: objects, TotalItems: total})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")

	// Limit request body size.
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize)

	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "invalid multipart form or file too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "missing \"file\" field in multipart form")
		return
	}
	defer file.Close()

	// Use provided name or fall back to uploaded filename.
	name := r.FormValue("name")
	if name == "" {
		name = header.Filename
	}
	if name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "file name is required")
		return
	}

	// Detect content type from extension, fall back to header.
	contentType := mime.TypeByExtension(filepath.Ext(name))
	if contentType == "" {
		contentType = header.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var userID *string
	if claims := auth.ClaimsFromContext(r.Context()); claims != nil {
		userID = &claims.Subject
	}

	obj, err := h.svc.Upload(r.Context(), bucket, name, contentType, userID, file)
	if err != nil {
		if errors.Is(err, ErrInvalidBucket) || errors.Is(err, ErrInvalidName) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		h.logger.Error("upload error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, obj)
}

func (h *Handler) handleServe(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")

	// Check for signed URL params.
	if sig := r.URL.Query().Get("sig"); sig != "" {
		exp := r.URL.Query().Get("exp")
		if !h.svc.ValidateSignedURL(bucket, name, exp, sig) {
			httputil.WriteError(w, http.StatusForbidden, "invalid or expired signed URL")
			return
		}
		// Signed URL is valid — serve the file without further auth checks.
		h.serveFile(w, r, bucket, name)
		return
	}

	h.serveFile(w, r, bucket, name)
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, bucket, name string) {
	reader, obj, err := h.svc.Download(r.Context(), bucket, name)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("download error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", obj.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.Size, 10))
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, reader)
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "*")

	if err := h.svc.DeleteObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("delete error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type signRequest struct {
	ExpiresIn int `json:"expiresIn"` // seconds, default 3600
}

type signResponse struct {
	URL string `json:"url"`
}

func (h *Handler) handleSign(w http.ResponseWriter, r *http.Request) {
	bucket := chi.URLParam(r, "bucket")
	name := chi.URLParam(r, "name")

	var req signRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	expiry := time.Duration(req.ExpiresIn) * time.Second
	if expiry <= 0 {
		expiry = time.Hour
	}
	if expiry > 7*24*time.Hour {
		httputil.WriteError(w, http.StatusBadRequest, "expiresIn must not exceed 604800 (7 days)")
		return
	}

	// Verify object exists.
	if _, err := h.svc.GetObject(r.Context(), bucket, name); err != nil {
		if errors.Is(err, ErrNotFound) {
			httputil.WriteError(w, http.StatusNotFound, "file not found")
			return
		}
		h.logger.Error("sign error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	token := h.svc.SignURL(bucket, name, expiry)
	url := "/api/storage/" + bucket + "/" + name + "?" + token
	httputil.WriteJSON(w, http.StatusOK, signResponse{URL: url})
}
