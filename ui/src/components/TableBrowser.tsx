import { useEffect, useState, useCallback, useRef } from "react";
import type { Table, ListResponse } from "../types";
import { getRows, createRecord, updateRecord, deleteRecord, ApiError } from "../api";
import { RecordForm } from "./RecordForm";
import {
  ChevronUp,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Search,
  Plus,
  Pencil,
  Trash2,
  X,
} from "lucide-react";

const PER_PAGE = 20;

type Modal =
  | { kind: "none" }
  | { kind: "create" }
  | { kind: "edit"; row: Record<string, unknown> }
  | { kind: "detail"; row: Record<string, unknown> }
  | { kind: "delete"; row: Record<string, unknown> };

interface TableBrowserProps {
  table: Table;
}

export function TableBrowser({ table }: TableBrowserProps) {
  const [data, setData] = useState<ListResponse | null>(null);
  const [page, setPage] = useState(1);
  const [sort, setSort] = useState<string | null>(null);
  const [filter, setFilter] = useState("");
  const [appliedFilter, setAppliedFilter] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const prevTableRef = useRef(table.name);

  const isWritable = table.kind === "table" || table.kind === "partitioned_table";
  const hasPK = table.primaryKey.length > 0;

  // Reset state when table changes.
  useEffect(() => {
    if (prevTableRef.current !== table.name) {
      setPage(1);
      setSort(null);
      setFilter("");
      setAppliedFilter("");
      setModal({ kind: "none" });
      prevTableRef.current = table.name;
    }
  }, [table.name]);

  const fetchData = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await getRows(table.name, {
        page,
        perPage: PER_PAGE,
        sort: sort || undefined,
        filter: appliedFilter || undefined,
      });
      setData(result);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Failed to load data");
      setData(null);
    } finally {
      setLoading(false);
    }
  }, [table.name, page, sort, appliedFilter]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const toggleSort = useCallback(
    (col: string) => {
      setSort((prev) => {
        if (prev === `+${col}` || prev === col) return `-${col}`;
        return `+${col}`;
      });
      setPage(1);
    },
    [],
  );

  const handleFilterSubmit = useCallback(() => {
    setAppliedFilter(filter);
    setPage(1);
  }, [filter]);

  const pkId = useCallback(
    (row: Record<string, unknown>): string => {
      return table.primaryKey.map((k) => String(row[k])).join(",");
    },
    [table.primaryKey],
  );

  const handleCreate = useCallback(
    async (formData: Record<string, unknown>) => {
      await createRecord(table.name, formData);
      setModal({ kind: "none" });
      fetchData();
    },
    [table.name, fetchData],
  );

  const handleUpdate = useCallback(
    async (formData: Record<string, unknown>) => {
      if (modal.kind !== "edit") return;
      const id = pkId(modal.row);
      await updateRecord(table.name, id, formData);
      setModal({ kind: "none" });
      fetchData();
    },
    [table.name, modal, pkId, fetchData],
  );

  const handleDelete = useCallback(async () => {
    if (modal.kind !== "delete") return;
    const id = pkId(modal.row);
    await deleteRecord(table.name, id);
    setModal({ kind: "none" });
    fetchData();
  }, [table.name, modal, pkId, fetchData]);

  const columns = table.columns;

  return (
    <div className="flex flex-col h-full">
      {/* Toolbar */}
      <div className="px-4 py-2 border-b flex items-center gap-2 bg-gray-50">
        <Search className="w-4 h-4 text-gray-400" />
        <input
          type="text"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleFilterSubmit()}
          placeholder="Filter... e.g. status='active' && age>25"
          className="flex-1 bg-transparent text-sm outline-none placeholder-gray-400"
        />
        {filter && (
          <button
            onClick={() => {
              setFilter("");
              setAppliedFilter("");
              setPage(1);
            }}
            className="text-gray-400 hover:text-gray-600"
          >
            <X className="w-4 h-4" />
          </button>
        )}
        <button
          onClick={handleFilterSubmit}
          className="px-3 py-1 text-xs bg-gray-200 hover:bg-gray-300 rounded font-medium"
        >
          Apply
        </button>
        {isWritable && (
          <button
            onClick={() => setModal({ kind: "create" })}
            className="ml-2 px-3 py-1 text-xs bg-gray-900 text-white hover:bg-gray-800 rounded font-medium inline-flex items-center gap-1"
          >
            <Plus className="w-3.5 h-3.5" />
            New
          </button>
        )}
      </div>

      {/* Error */}
      {error && (
        <div className="px-4 py-2 bg-red-50 text-red-600 text-sm border-b">
          {error}
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <table className="w-full text-sm">
          <thead className="bg-gray-50 sticky top-0">
            <tr>
              {columns.map((col) => (
                <th
                  key={col.name}
                  onClick={() => toggleSort(col.name)}
                  className="px-4 py-2 text-left font-medium text-gray-600 border-b cursor-pointer hover:bg-gray-100 whitespace-nowrap select-none"
                >
                  <span className="inline-flex items-center gap-1">
                    {col.name}
                    {col.isPrimaryKey && (
                      <span className="text-blue-500 text-xs">PK</span>
                    )}
                    <SortIcon sort={sort} col={col.name} />
                  </span>
                </th>
              ))}
              {isWritable && hasPK && (
                <th className="px-4 py-2 border-b w-20" />
              )}
            </tr>
          </thead>
          <tbody>
            {loading && !data && (
              <tr>
                <td
                  colSpan={columns.length + (isWritable && hasPK ? 1 : 0)}
                  className="px-4 py-8 text-center text-gray-400"
                >
                  Loading...
                </td>
              </tr>
            )}
            {data?.items.length === 0 && (
              <tr>
                <td
                  colSpan={columns.length + (isWritable && hasPK ? 1 : 0)}
                  className="px-4 py-8 text-center text-gray-400"
                >
                  No rows found
                </td>
              </tr>
            )}
            {data?.items.map((row, i) => (
              <tr
                key={i}
                onClick={() => setModal({ kind: "detail", row })}
                className="border-b hover:bg-blue-50 cursor-pointer group"
              >
                {columns.map((col) => (
                  <td
                    key={col.name}
                    className="px-4 py-2 whitespace-nowrap max-w-xs truncate"
                  >
                    <CellValue value={row[col.name]} />
                  </td>
                ))}
                {isWritable && hasPK && (
                  <td className="px-2 py-2 whitespace-nowrap">
                    <span className="opacity-0 group-hover:opacity-100 inline-flex gap-1">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setModal({ kind: "edit", row });
                        }}
                        className="p-1 hover:bg-gray-200 rounded text-gray-500 hover:text-gray-700"
                        title="Edit"
                      >
                        <Pencil className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setModal({ kind: "delete", row });
                        }}
                        className="p-1 hover:bg-red-100 rounded text-gray-500 hover:text-red-600"
                        title="Delete"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </span>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Pagination */}
      {data && (
        <div className="px-4 py-2 border-t bg-gray-50 flex items-center justify-between text-sm text-gray-500">
          <span>
            {data.totalItems} row{data.totalItems !== 1 ? "s" : ""}
          </span>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
            >
              <ChevronLeft className="w-4 h-4" />
            </button>
            <span>
              {page} / {data.totalPages || 1}
            </span>
            <button
              onClick={() => setPage((p) => Math.min(data.totalPages, p + 1))}
              disabled={page >= data.totalPages}
              className="p-1 rounded hover:bg-gray-200 disabled:opacity-30"
            >
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      {/* Create form */}
      {modal.kind === "create" && (
        <RecordForm
          columns={columns}
          primaryKey={table.primaryKey}
          onSubmit={handleCreate}
          onClose={() => setModal({ kind: "none" })}
          mode="create"
        />
      )}

      {/* Edit form */}
      {modal.kind === "edit" && (
        <RecordForm
          columns={columns}
          primaryKey={table.primaryKey}
          initialData={modal.row}
          onSubmit={handleUpdate}
          onClose={() => setModal({ kind: "none" })}
          mode="edit"
        />
      )}

      {/* Row detail drawer */}
      {modal.kind === "detail" && (
        <RowDetail
          row={modal.row}
          columns={columns}
          isWritable={isWritable && hasPK}
          onClose={() => setModal({ kind: "none" })}
          onEdit={() => setModal({ kind: "edit", row: modal.row })}
          onDelete={() => setModal({ kind: "delete", row: modal.row })}
        />
      )}

      {/* Delete confirmation */}
      {modal.kind === "delete" && (
        <DeleteConfirm
          row={modal.row}
          primaryKey={table.primaryKey}
          tableName={table.name}
          onConfirm={handleDelete}
          onCancel={() => setModal({ kind: "none" })}
        />
      )}
    </div>
  );
}

function SortIcon({ sort, col }: { sort: string | null; col: string }) {
  if (sort === `+${col}` || sort === col)
    return <ChevronUp className="w-3 h-3 text-blue-500" />;
  if (sort === `-${col}`)
    return <ChevronDown className="w-3 h-3 text-blue-500" />;
  return <ChevronUp className="w-3 h-3 text-transparent" />;
}

function CellValue({ value }: { value: unknown }) {
  if (value === null || value === undefined)
    return <span className="text-gray-300 italic">null</span>;
  if (typeof value === "boolean")
    return <span className={value ? "text-green-600" : "text-gray-400"}>{String(value)}</span>;
  if (typeof value === "object")
    return (
      <span className="text-gray-500 font-mono text-xs">
        {JSON.stringify(value)}
      </span>
    );
  return <>{String(value)}</>;
}

function RowDetail({
  row,
  columns,
  isWritable,
  onClose,
  onEdit,
  onDelete,
}: {
  row: Record<string, unknown>;
  columns: { name: string; type: string }[];
  isWritable: boolean;
  onClose: () => void;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="fixed inset-0 z-50 flex justify-end">
      <div className="absolute inset-0 bg-black/20" onClick={onClose} />
      <div className="relative w-full max-w-lg bg-white shadow-lg overflow-y-auto">
        <div className="px-6 py-4 border-b flex items-center justify-between sticky top-0 bg-white">
          <h2 className="font-semibold">Row Detail</h2>
          <div className="flex items-center gap-1">
            {isWritable && (
              <>
                <button
                  onClick={onEdit}
                  className="p-1.5 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700"
                  title="Edit"
                >
                  <Pencil className="w-4 h-4" />
                </button>
                <button
                  onClick={onDelete}
                  className="p-1.5 hover:bg-red-50 rounded text-gray-500 hover:text-red-600"
                  title="Delete"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </>
            )}
            <button
              onClick={onClose}
              className="p-1.5 hover:bg-gray-100 rounded"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
        <div className="p-6 space-y-3">
          {columns.map((col) => (
            <div key={col.name}>
              <label className="text-xs font-medium text-gray-500 block mb-0.5">
                {col.name}{" "}
                <span className="text-gray-300 font-normal">{col.type}</span>
              </label>
              <div className="text-sm bg-gray-50 rounded px-3 py-2 font-mono break-all">
                {row[col.name] === null || row[col.name] === undefined ? (
                  <span className="text-gray-300 italic">null</span>
                ) : typeof row[col.name] === "object" ? (
                  JSON.stringify(row[col.name], null, 2)
                ) : (
                  String(row[col.name])
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function DeleteConfirm({
  row,
  primaryKey,
  tableName,
  onConfirm,
  onCancel,
}: {
  row: Record<string, unknown>;
  primaryKey: string[];
  tableName: string;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const pkDisplay = primaryKey.map((k) => `${k}=${row[k]}`).join(", ");

  const handleConfirm = async () => {
    setDeleting(true);
    setError(null);
    try {
      await onConfirm();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to delete");
      setDeleting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      <div className="absolute inset-0 bg-black/20" onClick={onCancel} />
      <div className="relative bg-white rounded-lg shadow-lg p-6 max-w-sm w-full mx-4">
        <h3 className="font-semibold text-gray-900 mb-2">Delete record?</h3>
        <p className="text-sm text-gray-600 mb-1">
          This will permanently delete the record from <strong>{tableName}</strong>.
        </p>
        <p className="text-xs text-gray-400 font-mono mb-4">{pkDisplay}</p>

        {error && (
          <div className="bg-red-50 border border-red-200 text-red-700 rounded px-3 py-2 text-sm mb-4">
            {error}
          </div>
        )}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 rounded"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={deleting}
            className="px-4 py-2 text-sm font-medium bg-red-600 text-white rounded hover:bg-red-700 disabled:opacity-50"
          >
            {deleting ? "Deleting..." : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}
