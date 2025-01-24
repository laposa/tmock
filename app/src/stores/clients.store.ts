import { useClientsApi, type Client } from '@/apis/useClientsApi';

export const useClientsStore = defineStore('clients', () => {

  const clientsApi = useClientsApi();
  
  const getClients = clientsApi.prepareGetClients();
  const getClientDetail = clientsApi.prepareGetClientDetail();

  const clients = computed(() => getClients.data.value ?? []);
  const detail = computed<Client | undefined>(() => getClientDetail.data.value?.client);

  async function reset() {
    getClients.reset();
    getClientDetail.reset();
  }

  async function load() {
    return getClients.load();
  }

  async function loadClientDetail(id: string) {
    return getClientDetail.load(id);
  }

  async function setClientEnabled(clientId: string, enabled: boolean) {
    return clientsApi.setClientEnabled(clientId, enabled);
  }

  async function setClientName(clientId: string, name: string) {
    return clientsApi.setClientName(clientId, name);
  }

  async function addNewClient(name: string) {
    return clientsApi.addNewClient(name);
  }

  return { clients, detail, reset, load, setClientEnabled, setClientName, loadClientDetail, addNewClient };
})
