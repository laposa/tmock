export const useClientsStore = defineStore('clients', () => {
  const clientsApi = useClientsApi();

  const getList = clientsApi.prepareGetList();

  const list = computed(() => getList.data.value);
  const detail = ref<Client | null>(null);

  async function reset() {
    getList.reset();
  }

  async function load() {
    return getList.load();
  }

  async function setDetail(client: Client | null) {
    detail.value = client;
  }

  return { list, detail, setDetail, reset, load };
});
