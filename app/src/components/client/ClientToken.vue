<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const props = defineProps<{ client: Client }>();

const uiStore = useUiStore();
const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const snackbarsStore = useSnackbarsStore();
const { snackbarWrapper } = useSnackbarWrapper();

const isLoading = ref(false);

async function copyToken() {
  if (!props.client.token) return;
  await navigator.clipboard.writeText(props.client.token);
  snackbarsStore.displaySnackbar({
    message: 'Token copied to clipboard',
    type: 'success',
  });
}

function close() {
  uiStore.closeDialog('client-token');
}

async function revoke() {
  isLoading.value = true;
  await snackbarWrapper(
    {
      errorTitle: `Failed to revoke token for ${props.client.name}`,
      successMessage: `Token revoked for <strong>${props.client.name}</strong>`,
    },
    async () => {
      await clientsApi.revokeToken(props.client.id);
      await clientsStore.load();
      close();
    },
  );
  isLoading.value = false;
}
</script>

<template>
  <ModalWindow id="client-token" title="Client Access Token">
    <div class="token-row">
      <v-text-field
        :model-value="props.client.token ?? ''"
        readonly
        hide-details
        variant="outlined"
        density="compact"
      />
      <v-btn
        icon="mdi-content-copy"
        variant="text"
        aria-label="Copy token"
        :disabled="!props.client.token"
        @click="copyToken"
      />
    </div>

    <template #actions>
      <v-btn
        color="red"
        :loading="isLoading"
        :disabled="isLoading || !props.client.token"
        @click="revoke"
      >
        Revoke
      </v-btn>
      <v-spacer />
      <v-btn @click="close">Close</v-btn>
    </template>
  </ModalWindow>
</template>

<style scoped>
.token-row {
  display: flex;
  align-items: center;
  gap: 8px;
}
</style>
