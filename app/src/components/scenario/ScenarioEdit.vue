<script setup lang="ts">
import type { Scenario } from '@/apis/useScenariosApi';

const scenariosStore = useScenariosStore();
const scenariosApi = useScenariosApi();
const uiStore = useUiStore();
const { snackbarWrapper } = useSnackbarWrapper();

const props = defineProps<{
  scenario: Scenario;
}>();

const name = ref(props.scenario.name);
const methodEnabled = ref(props.scenario.requestMethod && props.scenario.requestMethod !== '');
const method = ref(props.scenario.requestMethod);
const pathEnabled = ref(props.scenario.requestPath && props.scenario.requestPath !== '');
const path = ref(props.scenario.requestPath);
const conditionEnabled = ref(props.scenario.requestCondition && props.scenario.requestCondition !== '');
const condition = ref(props.scenario.requestCondition);

const confirmValue = ref('');
const confirmValueString = ref('');

// TODO error/success dictionary? would save a lot of code
async function changeScenarioName() {
  snackbarWrapper(
    {
      errorTitle: `Failed to rename scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> renamed to <strong>${name.value}</strong>`,
    },
    async () => {
      await scenariosApi.updateName(props.scenario.id.toString(), name.value);
      await scenariosStore.load();
    },
  );
}

async function changeScenarioMethodEnabled() {
  if(!methodEnabled.value && method.value !== '') {
    confirmValue.value = 'requestMethod';
    confirmValueString.value = 'Request Method';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeScenarioMethod() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request method of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request method changed to <strong>${method.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateMethod(props.scenario.id.toString(), method.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function changeScenarioPathEnabled() {
  if(!pathEnabled.value && path.value !== '') {
    confirmValue.value = 'requestPath';
    confirmValueString.value = 'Request Path';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeScenarioPath() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request path of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request path changed to <strong>${path.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updatePath(props.scenario.id.toString(), path.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function changeScenarioConditionEnabled() {
  if(!conditionEnabled.value && condition.value !== '') {
    confirmValue.value = 'requestCondition';
    confirmValueString.value = 'Request Condition';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeScenarioCondition() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request condition of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request condition changed to <strong>${condition.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateCondition(props.scenario.id.toString(), condition.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function confirmDelete() {
  
  switch(confirmValue.value) {
    case 'requestMethod':
      method.value = '';
      changeScenarioMethod();
      break;
    case 'requestPath':
      path.value = '';
      changeScenarioPath();
      break;
    case 'requestCondition':
      condition.value = '';
      changeScenarioCondition();
      break;
    default:
      return;
  }

  confirmValue.value = '';
}

async function cancelDelete() {
  
  switch(confirmValue.value) {
    case 'requestMethod':
      methodEnabled.value = true;
      break;
    case 'requestPath':
      pathEnabled.value = true;
      break;
    case 'requestCondition':
      conditionEnabled.value = true;
      break;
    default:
      return;
  }

  confirmValue.value = '';
}
</script>

<template>
  <ModalWindow id="scenario-edit" title="Edit Scenario">
    <span class="label">Name</span>
    <v-text-field v-model="name" @change="changeScenarioName()"></v-text-field>
    
    <span class="label">Request Conditions</span>
    <div class="row">
      <v-switch 
        :loading="confirmValue === 'requestMethod'"
        v-model="methodEnabled"
        color="indigo"
        :hide-details="true" 
        @update:model-value="changeScenarioMethodEnabled()">
      </v-switch>
      
      <v-select 
        label="Request Method" 
        v-if="methodEnabled"
        v-model="method" 
        :items="['GET', 'POST', 'PUT', 'DELETE']"
        :hide-details="true"
        @update:model-value="changeScenarioMethod()">
      </v-select>
      <div class="fake-label" v-else>Request Method</div>
    </div>

    <div class="row">
      <v-switch 
        :loading="confirmValue === 'requestPath'"
        v-model="pathEnabled"
        color="indigo"
        :hide-details="true" 
        @update:model-value="changeScenarioPathEnabled()">
      </v-switch>
      
      <v-text-field
        label="Request Path" 
        v-if="pathEnabled" 
        v-model="path" 
        @change="changeScenarioPath()" 
        :hide-details="true">
      </v-text-field>
      <div class="fake-label" v-else>Request Path</div>
    </div>

    <div class="row">
      <v-switch 
        :loading="confirmValue === 'requestCondition'"
        v-model="conditionEnabled"
        color="indigo"
        :hide-details="true" 
        @update:model-value="changeScenarioConditionEnabled()">
      </v-switch>
      
      <v-text-field
        label="Request Condition" 
        v-if="conditionEnabled" 
        v-model="condition" 
        @change="changeScenarioCondition()" 
        :hide-details="true">
      </v-text-field>
      <div class="fake-label" v-else>Request Condition</div>
    </div>
    

    <!-- 
    <template v-slot:actions>
      TODO: prompt to confirm delete
      <v-btn @click="deleteScenario()" color="red">Delete scenario</v-btn>
    </template> -->

    <ConfirmationDialog 
      @confirm="confirmDelete()"
      @discard="cancelDelete()">
      Are you sure you want to remove current <span class="highlight">{{ confirmValueString }}</span> value?
    </ConfirmationDialog>
  </ModalWindow>
</template>

<style scoped>
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
}

.label {
  font-weight: bold;
  margin: 1rem 0;
  display: block;
}

.fake-label {
  width: 100%;
  font-size: 16px;
  padding-left: 20px;
}
</style>
