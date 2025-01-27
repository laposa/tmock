import { useAxios } from '@vueuse/integrations/useAxios';

export type Client = {
  id: string;
  name: string;
  enabled: boolean;
  condition?: ClientCondition;
  scenarios?: number[];
};

export interface ClientCondition {
  and?: ClientCondition[];
  or?: ClientCondition[];
  not?: boolean;
  headerMatch?: { header: string; value: string };
  headerRegex?: { header: string; value: string };
  ip?: string;
  cidr?: string;
}

export const useClientsApi = () => {
  const tmockAxios = useTmockAxios();

  const prepareGetClients = () => {
    const axios = useAxios<Client[]>('/client', { method: 'GET' }, tmockAxios, {
      immediate: false,
    });

    const load = () => axios.execute(undefined, {});

    const reset = () => {
      axios.data.value = [];
    };

    const data = computed(() => {
      return axios.data.value ?? [];
    });

    return { ...axios, data, load, reset };
  };

  const prepareGetClientDetail = () => {
    const axios = useAxios<{ client: Client }>('/client/', { method: 'GET' }, tmockAxios, {
      immediate: false,
    });

    const load = (id: string) => axios.execute(`/client/${id}`, {});

    const reset = () => {
      axios.data.value = undefined;
    };

    const data = computed(() => {
      return axios.data.value;
    });

    return { ...axios, data, load, reset };
  };

  const setClientEnabled = async (clientId: string, enabled: boolean) => {
    await tmockAxios.patch(`/client/${clientId}`, {
      enabled: enabled,
    });
  };

  const setClientName = async (clientId: string, name: string) => {
    await tmockAxios.patch(`/client/${clientId}`, {
      name: name,
    });
  };

  const addNewClient = async (name: string) => {
    await tmockAxios.post(`/client`, {
      name: name,
      enabled: false,
    });
  };

  return {
    prepareGetClients,
    prepareGetClientDetail,
    setClientEnabled,
    setClientName,
    addNewClient,
  };
};
