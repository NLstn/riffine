import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import offlinePlugin from './offline-plugin.mjs';

export default defineConfig({
  plugins: [react(), offlinePlugin()],
  server: { host: '127.0.0.1', headers: { 'Cross-Origin-Opener-Policy': 'same-origin', 'Cross-Origin-Embedder-Policy': 'require-corp' } },
 preview: { headers: { 'Cross-Origin-Opener-Policy': 'same-origin', 'Cross-Origin-Embedder-Policy': 'require-corp' } },
});
