import { useAxios } from '@vueuse/integrations/useAxios';
import { useTmockApi } from './useAxiosInstance';
import { computed } from 'vue';

export type Client = {
  name: string;
  enabled: boolean;
  condition?: string; // other?
};

// export interface GetCampaignsResponse {
//   count: number;
//   items: Campaign[];
// }

export const useClientsApi = () => {

  const prepareGetClients = () => {
    // TODO response type? or just clients[]?
    // todo how to inject api url automatically?
    const axios = useAxios(
      'http://localhost:3000/client',
      { method: 'GET' },
      { immediate: false },
    );

    const load = () =>
      axios.execute(undefined, {});

    const reset = () => {
      axios.data.value = [];
    }

    const data = computed(() => {
      return axios.data.value ?? [];
    })

    return { ...axios, data, load, reset };
  };

  return {
    prepareGetClients,
  }
};
