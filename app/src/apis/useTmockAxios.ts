import axios from 'axios';

export const useTmockAxios = () => {
  const config = useConfig();

  const tmockApi = axios.create({
    baseURL: config.apiEndpoint,
    withCredentials: true,
  });

  tmockApi.interceptors.request.use((reqConfig) => {
    const authStore = useAuthStore();
    if (authStore.csrfToken) {
      reqConfig.headers['x-csrf-token'] = authStore.csrfToken;
    }
    return reqConfig;
  });

  return tmockApi;
};
