import { defineStore } from 'pinia'
import { useClientsApi, type Client } from '@/apis/useClientsApi';
import { computed } from 'vue';

export const useClientsStore = defineStore('clients', () => {

  const clientsApi = useClientsApi();
  
  const getClients = clientsApi.prepareGetClients();
  const getClientDetail = clientsApi.prepareGetClientDetail();

  const clients = computed(() => getClients.data.value ?? []);
  const detail = computed<null | Client>(() => getClientDetail.data.value.client);

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

  return { clients, detail, reset, load, setClientEnabled, setClientName, loadClientDetail };
})
