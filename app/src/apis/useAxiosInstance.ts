import { useConfig } from '@/composables/useConfig';
import axios from 'axios';

// create axios instances for both basic and auth requests
export const useAxiosInstance = (baseUrl?: string) => {
  const axiosInstance = axios.create();

  // set base url
  if (baseUrl) {
    axiosInstance.defaults.baseURL = baseUrl;
  }

  return {
    axiosAuth: axiosInstance,
  };
};

export const useTmockApi = () => {
  
  const config = useConfig();
  const { axiosAuth } = useAxiosInstance(config.apiEndpoint);

  return { axiosAuth };
};
