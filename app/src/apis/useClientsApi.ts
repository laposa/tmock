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
  const { axiosAuth } = useTmockApi();

  const prepareGetClients = () => {
    // TODO response type? or just clients[]?
    const axios = useAxios(
      '/client',
      { method: 'GET' },
      axiosAuth,
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

  const setClientEnabled = async (clientId: string, enabled: boolean) => {
    await axiosAuth.patch(`/client/${clientId}`, {
      enabled: enabled,
    });
  };

  return {
    prepareGetClients,
    setClientEnabled,
  }
};
