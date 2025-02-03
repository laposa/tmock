export const useClientsStore = defineStore('clients', () => {
  const clientsApi = useClientsApi();

  const getClients = clientsApi.prepareGetClients();

  const clients = computed(() => getClients.data.value ?? []);
  const detail = ref<Client | null>(null);

  async function reset() {
    getClients.reset();
  }

  async function load() {
    return getClients.load();
  }

  async function setDetail(client: Client | null) {
    detail.value = client;
  }

  return { clients, detail, setDetail, reset, load };
});
