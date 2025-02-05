export const useScenariosStore = defineStore('scenarios', () => {
  const scenariosApi = useScenariosApi();

  const getList = scenariosApi.prepareGetList();

  const list = computed(() => getList.data.value.services);

  async function reset() {
    getList.reset();
  }

  async function load() {
    return getList.load();
  }

  return { list, reset, load };
});
