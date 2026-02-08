import { useState, useCallback } from "react";
import type { SchemaCache, Table } from "../types";
import { TableBrowser } from "./TableBrowser";
import { SchemaView } from "./SchemaView";
import {
  Database,
  Table as TableIcon,
  Columns3,
  LogOut,
  RefreshCw,
} from "lucide-react";
import { cn } from "../lib/utils";

type View = "data" | "schema";

interface LayoutProps {
  schema: SchemaCache;
  onLogout: () => void;
  onRefresh: () => void;
}

export function Layout({ schema, onLogout, onRefresh }: LayoutProps) {
  const tables = Object.values(schema.tables).sort((a, b) =>
    `${a.schema}.${a.name}`.localeCompare(`${b.schema}.${b.name}`),
  );
  const [selected, setSelected] = useState<Table | null>(
    tables.length > 0 ? tables[0] : null,
  );
  const [view, setView] = useState<View>("data");

  const handleSelect = useCallback((t: Table) => {
    setSelected(t);
    setView("data");
  }, []);

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <aside className="w-60 border-r bg-white flex flex-col">
        <div className="px-4 py-3 border-b flex items-center gap-2">
          <Database className="w-4 h-4 text-gray-500" />
          <span className="font-semibold text-sm">AYB Admin</span>
        </div>

        <nav className="flex-1 overflow-y-auto py-2">
          {tables.length === 0 && (
            <p className="px-4 py-2 text-xs text-gray-400">No tables found</p>
          )}
          {tables.map((t) => {
            const key = `${t.schema}.${t.name}`;
            const isSelected =
              selected &&
              selected.schema === t.schema &&
              selected.name === t.name;
            return (
              <button
                key={key}
                onClick={() => handleSelect(t)}
                className={cn(
                  "w-full text-left px-4 py-1.5 text-sm flex items-center gap-2 hover:bg-gray-100",
                  isSelected && "bg-gray-100 font-medium",
                )}
              >
                <TableIcon className="w-3.5 h-3.5 text-gray-400 shrink-0" />
                <span className="truncate">
                  {t.schema !== "public" && (
                    <span className="text-gray-400">{t.schema}.</span>
                  )}
                  {t.name}
                </span>
              </button>
            );
          })}
        </nav>

        <div className="border-t p-2 flex gap-1">
          <button
            onClick={onRefresh}
            className="p-2 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            title="Refresh schema"
          >
            <RefreshCw className="w-4 h-4" />
          </button>
          <button
            onClick={onLogout}
            className="p-2 text-gray-400 hover:text-gray-600 rounded hover:bg-gray-100"
            title="Log out"
          >
            <LogOut className="w-4 h-4" />
          </button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 flex flex-col overflow-hidden">
        {selected ? (
          <>
            <header className="border-b px-6 py-3 flex items-center gap-4">
              <h1 className="font-semibold">
                {selected.schema !== "public" && (
                  <span className="text-gray-400">{selected.schema}.</span>
                )}
                {selected.name}
              </h1>
              <span className="text-xs text-gray-400 bg-gray-100 rounded px-2 py-0.5">
                {selected.kind}
              </span>

              <div className="ml-auto flex gap-1 bg-gray-100 rounded p-0.5">
                <button
                  onClick={() => setView("data")}
                  className={cn(
                    "px-3 py-1 text-xs rounded font-medium",
                    view === "data"
                      ? "bg-white shadow-sm text-gray-900"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  <TableIcon className="w-3.5 h-3.5 inline mr-1" />
                  Data
                </button>
                <button
                  onClick={() => setView("schema")}
                  className={cn(
                    "px-3 py-1 text-xs rounded font-medium",
                    view === "schema"
                      ? "bg-white shadow-sm text-gray-900"
                      : "text-gray-500 hover:text-gray-700",
                  )}
                >
                  <Columns3 className="w-3.5 h-3.5 inline mr-1" />
                  Schema
                </button>
              </div>
            </header>

            <div className="flex-1 overflow-auto">
              {view === "data" ? (
                <TableBrowser table={selected} />
              ) : (
                <SchemaView table={selected} />
              )}
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-gray-400 text-sm">
            Select a table from the sidebar
          </div>
        )}
      </main>
    </div>
  );
}
