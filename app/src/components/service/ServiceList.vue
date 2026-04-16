<script setup lang="ts">
const uiStore = useUiStore();
const servicesStore = useServicesStore();

servicesStore.load();

const services = computed(() => servicesStore.list ?? []);

function openServiceEdit(service: typeof services.value[number]) {
  servicesStore.setDetail(service);
  uiStore.openDialog('service-edit');
}
</script>

<template>
  <main>
    <v-table theme="dark">
      <thead>
        <tr>
          <th>Name</th>
          <th>Upstream URL</th>
          <th>Path</th>
        </tr>
      </thead>

      <tbody>
        <tr v-for="service in services" :key="service.path">
          <td>
            <span class="edit" @click="openServiceEdit(service)">{{ service.name }}</span>
          </td>
          <td>{{ service.upstreamUrl }}</td>
          <td>{{ service.path }}</td>
        </tr>
      </tbody>
    </v-table>
  </main>
</template>

<style scoped>
.v-table {
  border-radius: 10px;
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
