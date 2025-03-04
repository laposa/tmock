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

  const updateScenario = async (scenarioId: string, data: Scenario) => {
    await tmockAxios.patch(`/scenario/${scenarioId}`, data);
  };

  const deleteScenario = async (scenarioId: string) => {
    await tmockAxios.delete(`/scenario/${scenarioId}`);
  }

  const create = async (scenario: Omit<Scenario, 'id'>) => {
    await tmockAxios.post(`/scenario`, scenario);
  };

  return {
    prepareGetList, updateScenario, deleteScenario, create
  };
};
