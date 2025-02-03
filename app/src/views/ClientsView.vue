<script setup lang="ts">
const clientsStore = useClientsStore();
</script>

<template>
  <main>
    <div class="heading">
      <h1>Clients</h1>
      <ClientAdd></ClientAdd>
    </div>
    <ClientList></ClientList>
    <ClientEdit v-if="clientsStore.detail" :client="clientsStore.detail"></ClientEdit>
    <ClientConditionsEdit
      v-if="clientsStore.detail"
      :client="clientsStore.detail"
    ></ClientConditionsEdit>
  </main>
</template>

<style scoped>
.heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 1rem;
}
</style>
