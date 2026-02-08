// Schema types matching Go's schema.SchemaCache JSON output.

export interface SchemaCache {
  tables: Record<string, Table>;
  schemas: string[];
  builtAt: string;
}

export interface Table {
  schema: string;
  name: string;
  kind: string;
  comment?: string;
  columns: Column[];
  primaryKey: string[];
  foreignKeys?: ForeignKey[];
  indexes?: Index[];
  relationships?: Relationship[];
}

export interface Column {
  name: string;
  position: number;
  type: string;
  nullable: boolean;
  default?: string;
  comment?: string;
  isPrimaryKey: boolean;
  jsonType: string;
  enumValues?: string[];
}

export interface ForeignKey {
  constraintName: string;
  columns: string[];
  referencedSchema: string;
  referencedTable: string;
  referencedColumns: string[];
  onUpdate?: string;
  onDelete?: string;
}

export interface Index {
  name: string;
  isUnique: boolean;
  isPrimary: boolean;
  method: string;
  definition: string;
}

export interface Relationship {
  name: string;
  type: string;
  fromSchema: string;
  fromTable: string;
  fromColumns: string[];
  toSchema: string;
  toTable: string;
  toColumns: string[];
  fieldName: string;
}

// API list response envelope.
export interface ListResponse {
  items: Record<string, unknown>[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}
