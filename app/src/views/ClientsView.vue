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
          <th>Name</th>
          <th>Enabled</th>
          <th>Conditions</th>
        </tr>
      </thead>

      <!-- TODO active scenarios -->
      <!-- TODO: sort by name - data table ? -->
      <!-- ClientList as separate component? due to own actions and styling and stuff? -->
      <tbody>
        <tr v-for="client in clientsStore.clients" :key="client.id">
          <td>{{ client.name }}</td>
          <td>{{ client.enabled }}</td>
          <td>{{ client.conditions }}</td>
        </tr>
      </tbody>
    </v-table>
  </main>
</template>
