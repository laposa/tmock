import axios from 'axios';

export const useTmockAxios = () => {
  const config = useConfig();

  const tmockApi = axios.create();
  tmockApi.defaults.baseURL = config.apiEndpoint;

  if (config.apiKey) {
    tmockApi.defaults.headers.common['apikey'] = config.apiKey;
  }

  return tmockApi;
};
