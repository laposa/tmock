import { useConfig } from '@/composables/useConfig';
import axios from 'axios';

export const useTmockAxios = () => {
  const config = useConfig();

  const tmockApi = axios.create()
  axios.defaults.baseURL = config.apiEndpoint;

  if (config.apiKey) {
    axios.defaults.headers.common['apikey'] = config.apiKey;
  }

  return tmockApi
};
