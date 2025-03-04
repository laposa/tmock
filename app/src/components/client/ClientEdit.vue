<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const uiStore = useUiStore();
const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

const props = defineProps<{
  client: Client;
}>();

const name = ref(props.client.name);

function deleteClient() {
  uiStore.openDialog('confirmation-dialog');
}

async function confirmDelete() {
  snackbarWrapper(
    {
      errorTitle: `Failed to delete client ${props.client.name}`,
      successMessage: `Client <strong>${props.client.name}</strong> has been successfully deleted.`,
    },
    async () => {
      await clientsApi.deleteClient(props.client.id);
      await clientsStore.load();
      await uiStore.closeDialog('client-edit');
    },
  );
}

async function changeClientName() {
  snackbarWrapper(
    {
      errorTitle: `Failed to rename client ${props.client.name}`,
      successMessage: `Client <strong>${props.client.name}</strong> renamed to <strong>${name.value}</strong>`,
    },
    async () => {
      await clientsApi.updateName(props.client.id, name.value);
      await clientsStore.load();
    },
  );
}
</script>

<template>
  <ModalWindow id="client-edit" title="Edit Client">
    <v-text-field label="Name" v-model="name" @change="changeClientName()"></v-text-field>

    <template v-slot:actions>
      <v-btn @click="deleteClient()" color="red">Delete</v-btn>
      <v-spacer></v-spacer>
      <v-btn @click="uiStore.closeDialog('client-edit')">Close</v-btn>
    </template>

    <ConfirmationDialog 
      @confirm="confirmDelete()"
      @discard="uiStore.closeDialog('confirmation-dialog')">
        Are you sure you want to delete client <b>{{ props.client.name }}</b>?
    </ConfirmationDialog>
  </ModalWindow>
</template>
