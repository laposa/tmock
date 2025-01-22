import { useAxios } from '@vueuse/integrations/useAxios';
import { useTmockApi } from './useAxiosInstance';
import { computed } from 'vue';

export type Client = {
  id: string;
  name: string;
  enabled: boolean;
  condition?: string;
  scenarios?: number[];
};


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

  const prepareGetClientDetail = () => {
    const axios = useAxios(
      '/client/',
      { method: 'GET' },
      axiosAuth,
      { immediate: false },
    );

    const load = (id: string) =>
      axios.execute(`/client/${id}`, {}); //TODO is there a better way to call dynamic link?

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

  const setClientName = async (clientId: string, name: string) => {
    await axiosAuth.patch(`/client/${clientId}`, {
      name: name,
    });
  };

  const addNewClient = async (name: string) => {
    await axiosAuth.post(`/client`, {
      name: name,
      enabled: false
    });
  };

  return {
    prepareGetClients,
    prepareGetClientDetail,
    setClientEnabled,
    setClientName,
    addNewClient
  }
};
