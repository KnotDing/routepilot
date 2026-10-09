import {defineConfig} from 'vite';
// WKWebView's local-file origin cannot fetch CORS module scripts. This offline
// viewer has a single classic bundle and relative assets, with no network loads.
export default defineConfig({base:'./',plugins:[{name:'offline-notes-html',transformIndexHtml:{order:'post',handler:html=>html.replaceAll('type="module" ','defer ').replaceAll(' crossorigin','')}}],build:{outDir:'dist-native',emptyOutDir:true,rollupOptions:{input:'notes.html',output:{format:'iife'}}}});
