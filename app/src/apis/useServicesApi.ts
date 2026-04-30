import { useAxios } from '@vueuse/integrations/useAxios';

export type Service = {
  path: string;
  upstreamUrl: string;
  name: string;
};

export const useServicesApi = () => {
  const tmockAxios = useTmockAxios();

  const prepareGetList = () => {
    const axios = useAxios<Service[]>('/service', { method: 'GET' }, tmockAxios, {
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

  const create = async (data: { name: string; upstreamUrl: string; path: string }) => {
    await tmockAxios.post('/service', data);
  };

  const update = async (path: string, data: Partial<Service>) => {
    await tmockAxios.patch(`/service/${encodeURIComponent(path)}`, data);
  };

  const remove = async (path: string) => {
    await tmockAxios.delete(`/service/${encodeURIComponent(path)}`);
  };

  return {
    prepareGetList,
    create,
    update,
    remove,
  };
};
