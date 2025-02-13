<script setup lang="ts">
import type { ClientCondition } from '@/apis/useClientsApi';

const condition = defineModel<ClientCondition>({
  required: true,
});

const emit = defineEmits<{
  delete: [];
  'update:model-value': [ClientCondition];
}>();

const ruleName = computed(() => {
  if (condition.value.headerMatch !== undefined) {
    return 'Header';
  } else if (condition.value.headerRegex !== undefined) {
    return 'Header Regex';
  } else if (condition.value.ip !== undefined) {
    return 'IP Address';
  } else if (condition.value.cidr !== undefined) {
    return 'CIDR';
  }

  return '';
});

const className = computed(() => ruleName.value.toLowerCase().replace(' ', '-'));

const ruleOperator = computed(() => {
  const pre = condition.value.not ? 'Not ' : '';

  if (condition.value.headerRegex !== undefined) {
    return pre + 'Matches';
  } else if (
    condition.value.headerMatch !== undefined ||
    condition.value.ip !== undefined ||
    condition.value.cidr !== undefined
  ) {
    return pre + 'Equals';
  }

  return '';
});

const ruleValue = computed({
  get: () => {
    if (condition.value.headerMatch) {
      return condition.value.headerMatch.value;
    } else if (condition.value.headerRegex) {
      return condition.value.headerRegex.value;
    } else if (condition.value.ip) {
      return condition.value.ip;
    } else if (condition.value.cidr) {
      return condition.value.cidr;
    }

    return '';
  },
  set: (value: string) => {
    if (condition.value.headerMatch !== undefined) {
      condition.value.headerMatch.value = value;
    } else if (condition.value.headerRegex !== undefined) {
      condition.value.headerRegex.value = value;
    } else if (condition.value.ip !== undefined) {
      condition.value.ip = value;
    } else if (condition.value.cidr !== undefined) {
      condition.value.cidr = value;
    }

    emit('update:model-value', condition.value);
  },
});

const headerValue = computed({
  get: () => {
    if (condition.value.headerMatch) {
      return condition.value.headerMatch.header;
    } else if (condition.value.headerRegex) {
      return condition.value.headerRegex.header;
    }

    return '';
  },
  set: (value: string) => {
    if (condition.value.headerMatch !== undefined) {
      condition.value.headerMatch.header = value;
    } else if (condition.value.headerRegex !== undefined) {
      condition.value.headerRegex.header = value;
    }

    emit('update:model-value', condition.value);
  },
});
</script>

<template>
  <div class="condition-item">
    <v-chip class="rule-name" :class="className">{{ ruleName }}</v-chip>

    <ClientConditionItemInput
      v-if="condition.headerMatch || condition.headerRegex"
      v-model="headerValue"
    ></ClientConditionItemInput>

    <v-chip variant="elevated" @click="condition.not = !condition.not">{{ ruleOperator }}</v-chip>

    <ClientConditionItemInput v-model="ruleValue"></ClientConditionItemInput>

    <v-chip variant="elevated" class="delete" @click="emit('delete')">
      <v-icon>mdi-close</v-icon>
    </v-chip>
  </div>
</template>

<style scoped>
.condition-item {
  display: flex;
  align-items: center;
  margin: 10px 0;
}

.condition-item :deep(.v-chip) {
  margin-right: 10px;
}

.rule-name.header {
  background-color: #ffecb3;
}

.rule-name.header-regex {
  background-color: #ffccbc;
}

.rule-name.ip-address {
  background-color: #b2dfdb;
}

.rule-name.cidr {
  background-color: #c5cae9;
}

.delete {
  cursor: pointer;
  background-color: #f44336;
  color: white;
  opacity: 0;
}

.condition-item:hover .delete {
  opacity: 1;
}
</style>
