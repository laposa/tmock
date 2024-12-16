import './assets/main.css'
import '@mdi/font/css/materialdesignicons.css'

import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import { createVuetify } from 'vuetify'

const vuetify = createVuetify()

const app = createApp(App)

app.use(createPinia())
app.use(router)

app.use(vuetify).mount('#app')
