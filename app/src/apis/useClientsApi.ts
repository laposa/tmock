import { useAxios } from '@vueuse/integrations/useAxios';

export type Client = {
  id: string;
  name: string;
  enabled: boolean;
  condition?: ClientCondition;
  scenarios?: number[];
};

export type ClientCondition = {
  and?: ClientCondition[];
  or?: ClientCondition[];
  not?: boolean;
  headerMatch?: { header: string; value: string };
  headerRegex?: { header: string; value: string };
  ip?: string;
  cidr?: string;
};

export const useClientsApi = () => {
  const tmockAxios = useTmockAxios();

  const prepareGetList = () => {
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

  const updateEnabled = async (clientId: string, enabled: boolean) => {
    await tmockAxios.patch(`/client/${clientId}`, {
      enabled,
    });
  };

  const updateName = async (clientId: string, name: string) => {
    await tmockAxios.patch(`/client/${clientId}`, {
      name,
    });
  };

  const updateCondition = async (clientId: string, condition: ClientCondition) => {
    await tmockAxios.patch(`/client/${clientId}`, {
      condition,
    });
  };

  const create = async (name: string) => {
    await tmockAxios.post(`/client`, {
      name,
      enabled: false,
    });
  };

  return {
    prepareGetList,
    updateEnabled,
    updateName,
    create,
    updateCondition,
  };
};
