<script setup lang="ts">
const scenariosStore = useScenariosStore();
</script>

<template>
  <main>
    <h1>Scenarios</h1>
    <ScenarioList></ScenarioList>
    <ScenarioEdit 
      v-if="scenariosStore.detail" 
      :scenario="scenariosStore.detail">
    </ScenarioEdit>
  </main>
</template>
