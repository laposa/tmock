<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';

const clientsStore = useClientsStore();
const clientsApi = useClientsApi();
const { snackbarWrapper } = useSnackbarWrapper();

const props = defineProps<{
  client: Client;
}>();

const condition = ref(structuredClone(toRaw(props.client.condition) || { and: [] }));
const isSaving = ref(false);

async function saveClientCondition() {
  isSaving.value = true;

  await snackbarWrapper(
    {
      errorTitle: `Failed to update condition for client ${props.client.name}`,
      successMessage: `Condition for client <strong>${props.client.name}</strong> updated`,
    },
    async () => {
      await clientsApi.updateCondition(props.client.id, condition.value);
      await clientsStore.load();
    },
  );

  isSaving.value = false;
}
</script>

<template>
  <ModalWindow id="client-conditions" title="Edit Client Conditions">
    <ClientConditionGroup v-model="condition" :is-top-level="true" />

    <template #actions>
      <v-btn @click="saveClientCondition()" color="primary" :disabled="isSaving" :loading="isSaving"
        >Save</v-btn
      >
    </template>
  </ModalWindow>
</template>
