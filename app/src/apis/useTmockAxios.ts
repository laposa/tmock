import axios from 'axios';

export const useTmockAxios = () => {
  const config = useConfig();

  const tmockApi = axios.create({
    baseURL: config.apiEndpoint,
    withCredentials: true,
  });

  if (config.apiKey) {
    tmockApi.defaults.headers.common['apikey'] = config.apiKey;
  }

  tmockApi.interceptors.request.use((reqConfig) => {
    const authStore = useAuthStore();
    if (authStore.csrfToken) {
      reqConfig.headers['x-csrf-token'] = authStore.csrfToken;
    }
    return reqConfig;
  });

  return tmockApi;
};
