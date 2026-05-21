<script setup lang="ts">
import type { Service } from '@/apis/useServicesApi';

const uiStore = useUiStore();
const servicesStore = useServicesStore();
const servicesApi = useServicesApi();
const { snackbarWrapper } = useSnackbarWrapper();

const isLoading = ref(false);

const props = defineProps<{
  service: Service;
}>();

const name = ref(props.service.name);
const upstreamUrl = ref(props.service.upstreamUrl);
const path = ref(props.service.path);

function deleteService() {
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
      errorTitle: `Failed to delete service ${props.service.name}`,
      successMessage: `Service <strong>${props.service.name}</strong> has been successfully deleted.`,
    },
    async () => {
      await servicesApi.remove(props.service.path);
      await servicesStore.load();
      await uiStore.closeDialog('service-edit');
      isLoading.value = false;
    },
  );
}

async function saveService() {
  isLoading.value = true;
  snackbarWrapper(
    {
      errorTitle: `Failed to update service ${props.service.name}`,
      successMessage: `Service <strong>${props.service.name}</strong> updated`,
    },
    async () => {
      await servicesApi.update(props.service.path, {
        name: name.value,
        upstreamUrl: upstreamUrl.value,
        path: path.value,
      });
      await servicesStore.load();
      await uiStore.closeDialog('service-edit');
      isLoading.value = false;
    },
  );
}
</script>

<template>
  <ModalWindow id="service-edit" title="Edit Service">
    <v-text-field label="Name" v-model="name"></v-text-field>
    <v-text-field label="Upstream URL" v-model="upstreamUrl"></v-text-field>
    <v-text-field label="Path" v-model="path"></v-text-field>

    <template v-slot:actions>
      <v-btn
        color="red"
        :disabled="isLoading"
        :loading="isLoading"
        @click="deleteService()">
          Delete
      </v-btn>
      <v-spacer></v-spacer>
      <v-btn @click="uiStore.closeDialog('service-edit')">Close</v-btn>
      <v-btn
        :loading="isLoading"
        :disabled="isLoading"
        color="indigo"
        @click="saveService()">
          Save
      </v-btn>
    </template>

    <ConfirmationDialog
      @confirm="confirmDelete"
      @discard="cancelDelete">
        Are you sure you want to delete the service <b>{{ props.service.name }}</b>?
    </ConfirmationDialog>
  </ModalWindow>
</template>
