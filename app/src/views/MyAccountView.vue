<script setup lang="ts">
const authStore = useAuthStore();
const usersApi = useUsersApi();
const { snackbarWrapper } = useSnackbarWrapper();

const name = ref(authStore.user?.name ?? '');
const email = ref(authStore.user?.email ?? '');
const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const isLoading = ref(false);

const passwordMismatch = computed(() => {
  return newPassword.value && confirmPassword.value && newPassword.value !== confirmPassword.value;
});

async function saveProfile() {
  if (passwordMismatch.value) return;

  isLoading.value = true;
  await snackbarWrapper(
    { errorTitle: 'Failed to update profile', successMessage: 'Profile updated successfully' },
    async () => {
      const payload: Record<string, string> = {};
      if (name.value !== authStore.user?.name) payload.name = name.value;
      if (email.value !== authStore.user?.email) payload.email = email.value;
      if (newPassword.value) {
        payload.currentPassword = currentPassword.value;
        payload.newPassword = newPassword.value;
      }

      const updated = await usersApi.updateProfile(payload);
      authStore.setUser({ ...authStore.user!, ...updated });
      currentPassword.value = '';
      newPassword.value = '';
      confirmPassword.value = '';
    },
  );
  isLoading.value = false;
}
</script>

<template>
  <main>
    <h1 class="mb-6">My Account</h1>

    <v-card max-width="600" class="pa-6">
      <v-text-field
        v-model="name"
        label="Name"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />
      <v-text-field
        v-model="email"
        label="Email"
        type="email"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />

      <v-divider class="my-4" />
      <h3 class="mb-4">Change Password</h3>

      <v-text-field
        v-model="currentPassword"
        label="Current Password"
        type="password"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />
      <v-text-field
        v-model="newPassword"
        label="New Password"
        type="password"
        variant="outlined"
        density="comfortable"
        class="mb-2"
      />
      <v-text-field
        v-model="confirmPassword"
        label="Confirm New Password"
        type="password"
        variant="outlined"
        density="comfortable"
        :error-messages="passwordMismatch ? ['Passwords do not match'] : []"
        class="mb-4"
      />

      <v-btn
        color="indigo"
        :disabled="isLoading || !!passwordMismatch"
        :loading="isLoading"
        @click="saveProfile"
      >
        Save Changes
      </v-btn>
    </v-card>
  </main>
</template>
