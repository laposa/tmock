<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';
import { useClientsStore } from '@/stores/clients.store';
import { computed, ref } from 'vue';

const clientsStore = useClientsStore();

const client = computed(() => clientsStore.detail ?? {
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

<style scoped>
td:first-of-type, th:first-of-type {
  width: 60px;
}

.edit {
  color: var(--primary-medium);
  cursor: pointer;
}

.edit:hover {
  color: white;
}
</style>
