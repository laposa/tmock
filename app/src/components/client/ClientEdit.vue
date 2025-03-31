<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const uiStore = useUiStore();
const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

const isLoading = ref(false);

const props = defineProps<{
  client: Client;
}>();

const name = ref(props.client.name);

function deleteClient() {
  isLoading.value = true;
  uiStore.openDialog('confirmation-dialog');
}

function cancelDelete() {
  isLoading.value = false;
  uiStore.closeDialog('confirmation-dialog');
}

async function confirmDelete() {
  snackbarWrapper(
    {
      errorTitle: `Failed to delete client ${props.client.name}`,
      successMessage: `Client <strong>${props.client.name}</strong> has been successfully deleted.`,
    },
    async () => {
      await clientsApi.remove(props.client.id);
      await clientsStore.load();
      await uiStore.closeDialog('client-edit');
      isLoading.value = false;
    },
  );
}

async function saveClient() {
  isLoading.value = true;
  snackbarWrapper(
    {
      errorTitle: `Failed to rename client ${props.client.name}`,
      successMessage: `Client <strong>${props.client.name}</strong> renamed to <strong>${name.value}</strong>`,
    },
    async () => {
      await clientsApi.updateName(props.client.id, name.value);
      await clientsStore.load();
      await uiStore.closeDialog('client-edit');
      isLoading.value = true;
    },
  );
}
</script>

<template>
  <ModalWindow id="client-edit" title="Edit Client">
    <v-text-field label="Name" v-model="name"></v-text-field>

    <template v-slot:actions>
      <v-btn 
        color="red"
        :disabled="isLoading" 
        :loading="isLoading"
        @click="deleteClient()">
          Delete
      </v-btn>
      <v-spacer></v-spacer>
      <v-btn 
        :loading="isLoading"
        :disabled="isLoading"
        color="indigo"
        @click="saveClient()">
          Save
      </v-btn>
      <v-btn @click="uiStore.closeDialog('client-edit')">Close</v-btn>
    </template>

    <ConfirmationDialog 
      @confirm="confirmDelete"
      @discard="cancelDelete">
        Are you sure you want to delete the client <b>{{ props.client.name }}</b>?
    </ConfirmationDialog>
  </ModalWindow>
</template>
