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

  async function remove(scenarioId: string) {
    return scenariosApi.remove(scenarioId);
  }

  return { list, detail, setDetail, remove, reset, load };
});
