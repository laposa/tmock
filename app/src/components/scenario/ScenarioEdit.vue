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
const requestMethodEnabled = ref(props.scenario.requestMethod && props.scenario.requestMethod !== '');
const requestMethod = ref(props.scenario.requestMethod);
const requestPathEnabled = ref(props.scenario.requestPath && props.scenario.requestPath !== '');
const requestPath = ref(props.scenario.requestPath);
const requestConditionEnabled = ref(props.scenario.requestCondition && props.scenario.requestCondition !== '');
const requestCondition = ref(props.scenario.requestCondition);
const responseCodeEnabled = ref(props.scenario.responseCode ? true : false);
const responseCode = ref(props.scenario.responseCode);

const responseHeaders = ref(props.scenario.responseHeaders);
const responseHeadersEnabled = computed(() => responseHeaders.value !== null);

const confirmValue = ref('');
const confirmValueString = ref('');

function toggleResponseHeaders() {
  responseHeaders.value = responseHeaders.value ? null : {};
}

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

async function changeRequestMethodEnabled() {
  if(!requestMethodEnabled.value && requestMethod.value !== '') {
    confirmValue.value = 'requestMethod';
    confirmValueString.value = 'Request Method';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeRequestMethod() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request method of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request method changed to <strong>${requestMethod.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateRequestMethod(props.scenario.id.toString(), requestMethod.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function changeRequestPathEnabled() {
  if(!requestPathEnabled.value && requestPath.value !== '') {
    confirmValue.value = 'requestPath';
    confirmValueString.value = 'Request Path';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeRequestPath() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request path of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request path changed to <strong>${requestPath.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateRequestPath(props.scenario.id.toString(), requestPath.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function changeRequestConditionEnabled() {
  if(!requestConditionEnabled.value && requestCondition.value !== '') {
    confirmValue.value = 'requestCondition';
    confirmValueString.value = 'Request Condition';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeRequestCondition() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change request condition of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> request condition changed to <strong>${requestCondition.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateRequestCondition(props.scenario.id.toString(), requestCondition.value ?? '');
      await scenariosStore.load();
    },
  );
}

async function changeResponseCodeEnabled() {
  if(!responseCodeEnabled.value && responseCode.value !== null) {
    confirmValue.value = 'responseCode';
    confirmValueString.value = 'Response Code';
    uiStore.openDialog('confirmation-dialog');
  }
}

async function changeResponseCode() {
  snackbarWrapper(
    {
      errorTitle: `Failed to change response code of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> response code changed to <strong>${responseCode.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateResponseCode(props.scenario.id.toString(), responseCode.value ?? null);
      await scenariosStore.load();
    },
  );
}

async function changeResponseHeaders(newHeaders: Record<string, string>) {
  if(Object.keys(newHeaders ?? {}).length === 0) {
    responseHeaders.value = null;
  } else {
    responseHeaders.value = newHeaders;
  }
  snackbarWrapper(
    {
      errorTitle: `Failed to change response headers of scenario ${props.scenario.name}`,
      successMessage: `Scenario <strong>${props.scenario.name}</strong> response headers to <strong>${responseHeaders.value}</strong>`,
    }, 
    async () => {
      await scenariosApi.updateResponseHeaders(props.scenario.id.toString(), responseHeaders.value);
      await scenariosStore.load();
    },
  );
}

async function confirmDelete() {
  
  switch(confirmValue.value) {
    case 'requestMethod':
      requestMethod.value = '';
      changeRequestMethod();
      break;
    case 'requestPath':
      requestPath.value = '';
      changeRequestPath();
      break;
    case 'requestCondition':
      requestCondition.value = '';
      changeRequestCondition();
      break;
    case 'responseCode':
      responseCode.value = null;
      changeResponseCode();
      break;
    case 'responseHeaders':
      responseHeaders.value = null;
      changeResponseHeaders({});
      break;
    default:
      return;
  }

  confirmValue.value = '';
}

async function cancelDelete() {
  
  switch(confirmValue.value) {
    case 'requestMethod':
      requestMethodEnabled.value = true;
      break;
    case 'requestPath':
      requestPathEnabled.value = true;
      break;
    case 'requestCondition':
      requestConditionEnabled.value = true;
      break;
    case 'responseCode':
      responseCodeEnabled.value = true;
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
        v-model="requestMethodEnabled"
        color="indigo"
        :loading="confirmValue === 'requestMethod'"
        :hide-details="true" 
        @update:model-value="changeRequestMethodEnabled()">
      </v-switch>
      
      <v-select 
        label="Request Method" 
        v-if="requestMethodEnabled"
        v-model="requestMethod" 
        :items="['GET', 'POST', 'PUT', 'PATCH', 'DELETE']"
        :hide-details="true"
        @update:model-value="changeRequestMethod()">
      </v-select>
      <div class="fake-label" v-else>Request Method</div>
    </div>

    <div class="row">
      <v-switch 
        v-model="requestPathEnabled"
        color="indigo"
        :loading="confirmValue === 'requestPath'"
        :hide-details="true" 
        @update:model-value="changeRequestPathEnabled()">
      </v-switch>
      
      <v-text-field
        label="Request Path" 
        v-if="requestPathEnabled" 
        v-model="requestPath" 
        :hide-details="true"
        @change="changeRequestPath()">
      </v-text-field>
      <div class="fake-label" v-else>Request Path</div>
    </div>

    <div class="row">
      <v-switch 
        v-model="requestConditionEnabled"
        color="indigo"
        :loading="confirmValue === 'requestCondition'"
        :hide-details="true" 
        @update:model-value="changeRequestConditionEnabled()">
      </v-switch>
      
      <v-text-field
        label="Request Condition" 
        v-if="requestConditionEnabled" 
        v-model="requestCondition" 
        :hide-details="true"
        @change="changeRequestCondition()">
      </v-text-field>
      <div class="fake-label" v-else>Request Condition</div>
    </div>

    <span class="label">Response Modifications</span>
    <div class="row">
      <v-switch 
        :loading="confirmValue === 'responseCode'"
        v-model="responseCodeEnabled"
        color="indigo"
        :hide-details="true" 
        @update:model-value="changeResponseCodeEnabled()">
      </v-switch>
      
      <v-text-field
        label="Response Code" 
        v-if="responseCodeEnabled"
        v-model.number="responseCode" 
        :hide-details="true"
        type="number"
        @change="changeResponseCode()">
      </v-text-field>
      <div class="fake-label" v-else>Response Code</div>
    </div>
    
    <div class="row">
      <v-switch 
        :loading="confirmValue === 'responseHeaders'"
        :model-value="responseHeadersEnabled"
        @update:model-value="toggleResponseHeaders"
        color="indigo"
        :hide-details="true" /> 
      
      <div class="fake-label">
        Response Headers
      </div>
    </div>

    <ResponseHeadersEdit 
      v-if="responseHeadersEnabled && responseHeaders !== null" 
      v-model="responseHeaders"
    />
    
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
