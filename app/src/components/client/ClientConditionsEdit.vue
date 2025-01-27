<script setup lang="ts">
const clientsStore = useClientsStore();

const condition = ref<ClientCondition | null>(null);
watch(
  () => clientsStore.detail?.condition,
  (value) => {
    if (value) {
      condition.value = value;
    }
  },
  { immediate: true },
);
</script>

<template>
  <ModalWindow id="client-conditions" title="Edit Client Conditions">
    <ClientConditionGroup v-if="condition" v-model="condition" :is-top-level="true" />
  </ModalWindow>
</template>
