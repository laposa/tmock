<script setup lang="ts">
</script>

<template>
  <main>
    <div class="heading">
      <h1>Clients</h1>
      <ClientAdd></ClientAdd>
    </div>
    <ClientList></ClientList>
    <ClientEdit></ClientEdit>
    <ClientConditionsEdit></ClientConditionsEdit>
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
