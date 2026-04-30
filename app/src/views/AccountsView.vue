<script setup lang="ts">
import type { User } from '@/apis/useAuthApi';
import type { CreateUserPayload, UpdateUserPayload } from '@/apis/useUsersApi';

const usersApi = useUsersApi();
const uiStore = useUiStore();
const { snackbarWrapper } = useSnackbarWrapper();

const users = ref<User[]>([]);
const isLoading = ref(false);
const editingUser = ref<User | null>(null);

// Add user form
const newName = ref('');
const newEmail = ref('');
const newPassword = ref('');
const newAdmin = ref(false);
const isAdding = ref(false);

// Edit user form
const editName = ref('');
const editEmail = ref('');
const editPassword = ref('');
const editAdmin = ref(false);
const isSaving = ref(false);

async function loadUsers() {
  isLoading.value = true;
  await snackbarWrapper({ errorTitle: 'Failed to load users' }, async () => {
    users.value = await usersApi.getAll();
  });
  isLoading.value = false;
}

async function addUser() {
  isAdding.value = true;
  await snackbarWrapper(
    { errorTitle: 'Failed to create user', successMessage: 'User created successfully' },
    async () => {
      const payload: CreateUserPayload = {
        name: newName.value,
        email: newEmail.value,
        password: newPassword.value,
        admin: newAdmin.value,
      };
      await usersApi.create(payload);
      uiStore.closeDialog('user-add');
      newName.value = '';
      newEmail.value = '';
      newPassword.value = '';
      newAdmin.value = false;
      await loadUsers();
    },
  );
  isAdding.value = false;
}

function openEdit(user: User) {
  editingUser.value = user;
  editName.value = user.name;
  editEmail.value = user.email;
  editPassword.value = '';
  editAdmin.value = user.admin;
  uiStore.openDialog('user-edit');
}

async function saveEdit() {
  if (!editingUser.value) return;
  isSaving.value = true;
  await snackbarWrapper(
    { errorTitle: 'Failed to update user', successMessage: 'User updated successfully' },
    async () => {
      const payload: UpdateUserPayload = {};
      if (editName.value !== editingUser.value!.name) payload.name = editName.value;
      if (editEmail.value !== editingUser.value!.email) payload.email = editEmail.value;
      if (editPassword.value) payload.password = editPassword.value;
      if (editAdmin.value !== editingUser.value!.admin) payload.admin = editAdmin.value;

      await usersApi.update(editingUser.value!.id, payload);
      uiStore.closeDialog('user-edit');
      editingUser.value = null;
      await loadUsers();
    },
  );
  isSaving.value = false;
}

async function deleteUser(user: User) {
  if (!confirm(`Are you sure you want to delete ${user.name}?`)) return;

  await snackbarWrapper(
    { errorTitle: 'Failed to delete user', successMessage: 'User deleted successfully' },
    async () => {
      await usersApi.remove(user.id);
      await loadUsers();
    },
  );
}

onMounted(loadUsers);
</script>

<template>
  <main>
    <div class="heading">
      <h1>Accounts</h1>
      <v-btn color="indigo" @click="uiStore.openDialog('user-add')">Add User</v-btn>
    </div>

    <v-table v-if="users.length" class="mt-4">
      <thead>
        <tr>
          <th>Name</th>
          <th>Email</th>
          <th>Admin</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="user in users" :key="user.id">
          <td>{{ user.name }}</td>
          <td>{{ user.email }}</td>
          <td>
            <v-icon v-if="user.admin" color="green">mdi-check-circle</v-icon>
            <v-icon v-else color="grey">mdi-close-circle</v-icon>
          </td>
          <td>
            <v-btn size="small" variant="text" color="indigo" @click="openEdit(user)">
              Edit
            </v-btn>
            <v-btn size="small" variant="text" color="red" @click="deleteUser(user)">
              Delete
            </v-btn>
          </td>
        </tr>
      </tbody>
    </v-table>

    <v-progress-linear v-if="isLoading" indeterminate color="indigo" />

    <!-- Add User Dialog -->
    <ModalWindow id="user-add" title="Add New User">
      <v-text-field v-model="newName" label="Name" variant="outlined" density="comfortable" class="mb-2" />
      <v-text-field v-model="newEmail" label="Email" type="email" variant="outlined" density="comfortable" class="mb-2" />
      <v-text-field v-model="newPassword" label="Password" type="password" variant="outlined" density="comfortable" class="mb-2" />
      <v-switch v-model="newAdmin" label="Admin" color="indigo" />

      <template v-slot:actions>
        <v-btn
          color="indigo"
          :disabled="isAdding || !newName || !newEmail || !newPassword"
          :loading="isAdding"
          @click="addUser"
        >
          Create
        </v-btn>
        <v-btn @click="uiStore.closeDialog('user-add')">Close</v-btn>
      </template>
    </ModalWindow>

    <!-- Edit User Dialog -->
    <ModalWindow id="user-edit" title="Edit User">
      <v-text-field v-model="editName" label="Name" variant="outlined" density="comfortable" class="mb-2" />
      <v-text-field v-model="editEmail" label="Email" type="email" variant="outlined" density="comfortable" class="mb-2" />
      <v-text-field v-model="editPassword" label="New Password (leave blank to keep)" type="password" variant="outlined" density="comfortable" class="mb-2" />
      <v-switch v-model="editAdmin" label="Admin" color="indigo" />

      <template v-slot:actions>
        <v-btn
          color="indigo"
          :disabled="isSaving || !editName || !editEmail"
          :loading="isSaving"
          @click="saveEdit"
        >
          Save
        </v-btn>
        <v-btn @click="uiStore.closeDialog('user-edit')">Close</v-btn>
      </template>
    </ModalWindow>
  </main>
</template>

<style scoped>
.heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
</style>
