<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

const props = defineProps<{
  client: Client;
}>();

const name = ref(props.client.name);

async function changeClientName() {
  snackbarWrapper(
    {
      errorTitle: `Failed to rename client ${props.client.name}`,
      successMessage: `Client <strong>${props.client.name}</strong> renamed to <strong>${name.value}</strong>`,
    },
    async () => {
      await clientsApi.setClientName(props.client.id, name.value);
      await clientsStore.load();
    },
  );
}
</script>

<template>
  <ModalWindow id="client-edit" title="Edit Client">
    <v-text-field label="Name" v-model="name" @change="changeClientName()"></v-text-field>
  </ModalWindow>
</template>
