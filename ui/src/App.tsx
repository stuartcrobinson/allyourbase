import { useEffect, useState, useCallback } from "react";
import { getAdminStatus, getSchema, clearToken, ApiError } from "./api";
import type { SchemaCache } from "./types";
import { Login } from "./components/Login";
import { Layout } from "./components/Layout";

type AppState =
  | { kind: "loading" }
  | { kind: "login" }
  | { kind: "ready"; schema: SchemaCache };

export function App() {
  const [state, setState] = useState<AppState>({ kind: "loading" });
  const [error, setError] = useState<string | null>(null);

  const boot = useCallback(async () => {
    try {
      // Check if admin auth is required.
      const status = await getAdminStatus();
      if (status.auth && !localStorage.getItem("ayb_admin_token")) {
        setState({ kind: "login" });
        return;
      }

      // Load schema.
      const schema = await getSchema();
      setState({ kind: "ready", schema });
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        clearToken();
        setState({ kind: "login" });
        return;
      }
      setError(e instanceof Error ? e.message : "Unknown error");
    }
  }, []);

  useEffect(() => {
    boot();
  }, [boot]);

  const handleLogin = useCallback(() => {
    setState({ kind: "loading" });
    boot();
  }, [boot]);

  const handleLogout = useCallback(() => {
    clearToken();
    setState({ kind: "login" });
  }, []);

  const refreshSchema = useCallback(async () => {
    try {
      const schema = await getSchema();
      setState({ kind: "ready", schema });
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) {
        clearToken();
        setState({ kind: "login" });
      }
    }
  }, []);

  if (error) {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <div className="bg-red-50 border border-red-200 rounded-lg p-6 max-w-md">
          <h2 className="text-red-800 font-semibold mb-2">Connection Error</h2>
          <p className="text-red-600 text-sm">{error}</p>
          <button
            onClick={() => {
              setError(null);
              setState({ kind: "loading" });
              boot();
            }}
            className="mt-4 px-4 py-2 bg-red-600 text-white rounded text-sm hover:bg-red-700"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (state.kind === "loading") {
    return (
      <div className="flex items-center justify-center min-h-screen">
        <p className="text-gray-500">Loading...</p>
      </div>
    );
  }

  if (state.kind === "login") {
    return <Login onSuccess={handleLogin} />;
  }

  return (
    <Layout
      schema={state.schema}
      onLogout={handleLogout}
      onRefresh={refreshSchema}
    />
  );
}
