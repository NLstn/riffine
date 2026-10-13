export interface SavedProject { id: string; name: string; sourceName: string; savedAt: number; recoveryOf?: string }
export interface ProjectStorage {
 list(): Promise<SavedProject[]>;
 read(id: string): Promise<{document: string}>;
 remove(id: string): Promise<void>;
}
export async function deviceProjects(): Promise<ProjectStorage> {
 const module = await import(/* @vite-ignore */ new URL('/practice/project-store.mjs', location.origin).href);
 return module.projectStore;
}
