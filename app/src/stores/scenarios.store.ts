export const useScenariosStore = defineStore('scenarios', () => {
  const scenariosApi = useScenariosApi();

  const getList = scenariosApi.prepareGetList();

  const list = computed(() => getList.data.value.services);
  const detail = ref<Scenario | null>(null);

  async function reset() {
    getList.reset();
  }

  async function load() {
    return getList.load();
  }

  async function setDetail(scenario: Scenario | null) {
    detail.value = scenario;
  }

  async function deleteScenario(scenarioId: string) {
    return scenariosApi.deleteScenario(scenarioId);
  }

  return { list, detail, setDetail, deleteScenario, reset, load };
});
