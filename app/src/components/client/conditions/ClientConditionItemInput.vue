<script setup lang="ts">
const model = defineModel<string>({
  required: true,
});
</script>

<template>
  <v-text-field
    class="client-condition-item-input"
    v-model="model"
    density="compact"
    variant="solo-filled"
    :hide-details="true"
  ></v-text-field>
</template>

<style scoped>
.client-condition-item-input {
  width: 100%;
  max-width: 160px;
  margin-right: 10px;
}
</style>
