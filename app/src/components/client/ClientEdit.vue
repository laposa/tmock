<script setup lang="ts">
import { useClientsStore } from '@/stores/clients.store';

const clientsStore = useClientsStore();

const client = computed(() => clientsStore.detail ?? {
  id: '',
  name: '',
});

async function changeClientName(id: string, name: string) {
  await clientsStore.setClientName(id, name);
  clientsStore.load();
}
</script>

<template>
  <ModalWindow :id="'clientEdit'">

    <h2>Edit client</h2>
    <br>
    <v-text-field
      label="Name"
      v-model="client.name"
      @change="changeClientName(client.id, client.name)"
    ></v-text-field>

  </ModalWindow>
</template>
