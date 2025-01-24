import { fileURLToPath, URL } from 'node:url'

import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'
import Components from 'unplugin-vue-components/vite';
import AutoImport from 'unplugin-auto-import/vite';
import vuetify from 'vite-plugin-vuetify';
import { createHtmlPlugin } from 'vite-plugin-html';

// https://vite.dev/config/
export default defineConfig(({ command }) => {
  return {
    plugins: [
      vue(),
      vuetify(),
      vueDevTools(),
      Components(),
      AutoImport({
        imports: [
          'vue',
          'vue-router',
          'pinia',
        ]
      }),
      createHtmlPlugin({
        inject: {
          data: {
            MAIN_FILE: command === 'build' ? 'main.build.ts' : 'main.local.ts',
          }
        }
      }),
    ],
    resolve: {
      extensions: ['.ts', '.js'],
      alias: {
        '~': fileURLToPath(new URL('./', import.meta.url)),
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
  }

})
