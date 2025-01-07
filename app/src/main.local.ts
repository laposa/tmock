import { initTmockApp } from './init';

initTmockApp({
  apiEndpoint: import.meta.env.VITE_API_ENDPOINT,
  buildTimestamp: import.meta.env.VITE_BUILD_TIMESTAMP,
});
