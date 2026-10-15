// Vite supplies the deployment prefix; direct Node conformance tests use '/'.
export function appPath(path: string): string {
 return `${import.meta.env?.BASE_URL ?? '/'}${path}`;
}
