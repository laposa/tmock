<script setup lang="ts">
import { useClientsStore } from '@/stores/clients.store';

const clientsStore = useClientsStore();

clientsStore.load();
</script>

<template>
  <main>
    <h1>Clients Page</h1>

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
      <!-- ClientList as separate component? due to own actions and styling and stuff? -->
      <tbody>
        <tr v-for="client in clientsStore.clients" :key="client.id">
          <td>
            <v-switch 
              v-model="client.enabled" 
              color="indigo" 
              hide-details
              @change="clientsStore.setClientEnabled(client.id, client.enabled)"
            />
          </td>
          <td>{{ client.name }}</td>
          <td>{{ client.conditions }}</td>
          <td>{{ client.scenarios }}</td>
        </tr>
      </tbody>
    </v-table>
  </main>
</template>

<style scoped>
td:first-of-type, th:first-of-type {
  width: 60px;
}
</style>
