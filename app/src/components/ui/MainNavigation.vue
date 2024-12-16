<script setup lang="ts">
</script>

<template>
  <nav>
    <a href="/" class="logo">Testing Mock Proxy</a>
    <div class="menu">
      <RouterLink to="/clients">Clients</RouterLink>
      <RouterLink to="/scenarios">Scenarios</RouterLink>
    </div>
  </nav>
</template>

<style scoped>
nav {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 2rem;
}

.logo {
  font-size: 1.5rem;
  font-weight: bold;
  text-decoration: none;
}
</style>
