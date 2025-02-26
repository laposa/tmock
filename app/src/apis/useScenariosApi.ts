import { useAxios } from '@vueuse/integrations/useAxios';

export type Scenario = {
  id: number;
  name: string;
  service: string;
  requestMethod: string | null;
  requestPath: string | null;
  requestCondition: string | null;
  responseCode: number | null;
  responseHeaders: Record<string, string> | null;
  responseBody: string | null;
};

export type Service = {
  path: string;
  scenarios: Scenario[];
};

export type ScenariosListResponse = {
  services: Service[];
};

export const useScenariosApi = () => {
  const tmockAxios = useTmockAxios();

  const prepareGetList = () => {
    const axios = useAxios<ScenariosListResponse>('/scenario', { method: 'GET' }, tmockAxios, {
      immediate: false,
    });

    const load = () => axios.execute(undefined, {});

    const reset = () => {
      axios.data.value = {
        services: [],
      };
    };

    const data = computed(() => {
      return (
        axios.data.value ?? {
          services: [],
        }
      );
    });

    return { ...axios, data, load, reset };
  };

  const updateName = async (scenarioId: string, name: string) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      name,
    });
  };

  const updateRequestMethod = async (scenarioId: string, requestMethod: string) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      requestMethod,
    });
  };

  const updateRequestPath = async (scenarioId: string, requestPath: string) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      requestPath,
    });
  };

  const updateRequestCondition = async (scenarioId: string, requestCondition: string) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      requestCondition,
    });
  };

  const updateResponseCode = async (scenarioId: string, responseCode: number | null) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      responseCode,
    });
  };

  const updateResponseHeaders = async (scenarioId: string, responseHeaders: Record<string, string> | null) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, {
      responseHeaders,
    });
  };

  return {
    prepareGetList, updateName, updateRequestMethod, updateRequestPath, updateRequestCondition, updateResponseCode, updateResponseHeaders
  };
};
