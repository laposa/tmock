import { initTmockApp } from './init';

initTmockApp({
  apiEndpoint: import.meta.env.VITE_API_ENDPOINT,
  apiKey: import.meta.env.VITE_API_KEY,
});
