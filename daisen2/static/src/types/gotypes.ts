export interface ComponentMetadata {
  name: string;
  type?: string;
  parent?: string;
  children?: string[];
  fields?: Record<string, unknown>;
}
