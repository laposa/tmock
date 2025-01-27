<script setup lang="ts">
const clientsStore = useClientsStore();

const client = computed(
  () =>
    clientsStore.detail ?? {
      id: '',
      name: '',
    },
);

async function changeClientName(id: string, name: string) {
  await clientsStore.setClientName(id, name);
  clientsStore.load();
}
</script>

<template>
  <ModalWindow id="client-edit" title="Edit Client">
    <v-text-field
      label="Name"
      v-model="client.name"
      @change="changeClientName(client.id, client.name)"
    ></v-text-field>
  </ModalWindow>
</template>
