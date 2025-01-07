import { defineStore } from 'pinia'
import { useClientsApi } from '@/apis/useClientsApi';
import { computed } from 'vue';

export const useClientsStore = defineStore('clients', () => {

  const clientsApi = useClientsApi();
  
  const getClients = clientsApi.prepareGetClients();

  const clients = computed(() => getClients.data.value ?? []);

  async function reset() {
    getClients.reset();
  }

  async function load() {
    return getClients.load();
  }

  return { clients, reset, load }
})
