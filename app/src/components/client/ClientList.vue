<script setup lang="ts">
import type { Client } from '@/apis/useClientsApi';
import { useClientsStore } from '@/stores/clients.store';
import { computed, ref } from 'vue';

const clientsStore = useClientsStore();
clientsStore.load();

const clients = computed(() => clientsStore.clients ?? []);

function openClientEdit(clientId: string) {
  const modal = document.querySelector('#clientEdit') as HTMLDialogElement;
  modal.showModal();
  clientsStore.loadClientDetail(clientId);
}
</script>

<template>
  <main>
    <v-table theme="dark">
      <thead>
        <tr>
          <th></th>
          <th>Name</th>
          <th>Conditions</th>
          <th>Scenarios</th>
        </tr>
      </thead>

      <!-- TODO active scenarios -->
      <!-- TODO: sort by name - data table ? -->
      <tbody>
        <tr v-for="client in clients" :key="client.id">
          <td>
            <v-switch 
              v-model="client.enabled" 
              color="indigo" 
              hide-details
              @change="clientsStore.setClientEnabled(client.id, client.enabled)"
            />
          </td>
          <td>
            <span class="edit" @click="openClientEdit(client.id)">{{ client.name }}</span>
          </td>
          <td>{{ client.conditions }}</td>
          <td>{{ client.scenarios }}</td>
        </tr>
      </tbody>
    </v-table>

    <!-- do client editu si nastavím jako model current client nebo tak něco jako computed a ten budu měnit pomoci kliku na to jméno společně s aktivací toho dialogu -->
    <!-- použít native dialog? dá se s ním pak pořešit více dialogů a tak? -->

    <ClientEdit></ClientEdit>
  </main>
</template>

<style scoped>

.v-table {
  border-radius: 10px;
}

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
