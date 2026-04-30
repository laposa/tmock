<script setup lang="ts">
const authStore = useAuthStore();
const authApi = useAuthApi();
const router = useRouter();

async function handleLogout() {
  try {
    await authApi.logout();
  } catch {
    // ignore logout errors
  }
  authStore.clearAuth();
  router.push('/login');
}
</script>

<template>
  <nav>
    <!-- TODO - play logo animation on loading? -->
    <RouterLink to="/" class="logo"><TmockLogoSvg></TmockLogoSvg><span>Tmock</span></RouterLink>
    <div class="menu">
      <RouterLink to="/services">Services</RouterLink>
      <RouterLink to="/">Clients</RouterLink>
      <RouterLink to="/scenarios">Scenarios</RouterLink>
      <RouterLink v-if="authStore.isAdmin" to="/accounts">Accounts</RouterLink>
      <RouterLink to="/my-account">My Account</RouterLink>
      <a href="#" @click.prevent="handleLogout">Logout</a>
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
  font-size: 1rem;
}

.logo {
  display: flex;
  align-items: center;
}

.logo svg {
  width: 50px;
  height: 50px;
  margin-right: 0.5rem;
}

.logo span {
  font-size: 2rem;
  font-weight: 500;
  letter-spacing: 1px;
  margin-bottom: 2px;
}

.menu {
  display: flex;
  gap: 20px;
}

.menu a:hover {
  filter: brightness(1.5);
  background-color: none;
}
</style>
