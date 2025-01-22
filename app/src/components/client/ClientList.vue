<script setup lang="ts">
import { openModalWindow } from '@/helpers';
import { useClientsStore } from '@/stores/clients.store';
import { computed } from 'vue';

const clientsStore = useClientsStore();
clientsStore.load();

const clients = computed(() => clientsStore.clients ?? []);

function openClientEdit(clientId: string) {
  openModalWindow('clientEdit');
  clientsStore.loadClientDetail(clientId);
}

function openClientConditionsEdit(clientId: string) {
  openModalWindow('clientConditionsEdit');
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
          <td><span class="edit" @click="openClientConditionsEdit(client.id)">Conditions</span></td>
          <!-- TODO scenario chips when scenarios are implemented -->
          <td>{{ client.scenarios }}</td>
        </tr>
      </tbody>
    </v-table>
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
  color: var(--primary);
  cursor: pointer;
  transition: 0.3s; 
}

.edit:hover {
  color: white;
}
</style>
