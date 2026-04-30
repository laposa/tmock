import type { Service } from '@/apis/useServicesApi';

export const useServicesStore = defineStore('services', () => {
  const servicesApi = useServicesApi();

  const getList = servicesApi.prepareGetList();

  const list = computed(() => getList.data.value);
  const detail = ref<Service | null>(null);

  async function reset() {
    getList.reset();
  }

  async function load() {
    return getList.load();
  }

  async function setDetail(service: Service | null) {
    detail.value = service;
  }

  return { list, detail, setDetail, reset, load };
});
