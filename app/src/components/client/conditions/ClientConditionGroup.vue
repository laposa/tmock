<script setup lang="ts">
import type { ClientCondition } from '@/apis/useClientsApi';

const condition = defineModel<ClientCondition>({
  required: true,
});

const emit = defineEmits<{
  delete: [];
  'update:model-value': [ClientCondition];
}>();

withDefaults(defineProps<{ isTopLevel?: boolean }>(), { isTopLevel: false });

const subConditions = computed({
  get: () => {
    if (condition.value.and) {
      return condition.value.and;
    } else if (condition.value.or) {
      return condition.value.or;
    }
    return [];
  },
  set: (value: ClientCondition[]) => {
    if (condition.value.and) {
      condition.value.and = value;
    } else if (condition.value.or) {
      condition.value.or = value;
    }

    emit('update:model-value', condition.value);
  },
});

function switchBoolean() {
  if (condition.value.and) {
    condition.value.or = condition.value.and;
    delete condition.value.and;
  } else if (condition.value.or) {
    condition.value.and = condition.value.or;
    delete condition.value.or;
  }

  emit('update:model-value', condition.value);
}

function addCondition(type: keyof ClientCondition) {
  let newCondition: ClientCondition = { [type]: '' };
  if (type === 'and' || type === 'or') {
    newCondition = { [type]: [] };
  } else if (type === 'headerMatch' || type === 'headerRegex') {
    newCondition = { [type]: { header: '', value: '' } };
  }

  subConditions.value = [...subConditions.value, newCondition];
}

function updateCondition(index: number, value: ClientCondition) {
  subConditions.value = subConditions.value.map((subCondition, i) =>
    i === index ? value : subCondition,
  );
}

function removeCondition(index: number) {
  subConditions.value = subConditions.value.filter((_, i) => i !== index);
}
</script>

<template>
  <div class="condition-group">
    <v-chip
      :variant="condition.not ? 'elevated' : 'plain'"
      class="condition-not"
      :class="{ active: condition.not }"
      @click="condition.not = !condition.not"
      >Not</v-chip
    >

    <v-chip class="parenthesis">(</v-chip>

    <div class="sub-conditions">
      <div v-for="(subCondition, index) in subConditions" :key="index">
        <ClientConditionGroup
          v-if="subCondition.and || subCondition.or"
          :model-value="subCondition"
          @update:model-value="(val) => updateCondition(index, val)"
          @delete="removeCondition(index)"
        />
        <ClientConditionItem
          v-else
          :model-value="subCondition"
          @update:model-value="(val) => updateCondition(index, val)"
          @delete="removeCondition(index)"
        />

        <v-chip
          v-if="index < subConditions.length - 1"
          variant="elevated"
          class="boolean"
          :class="{ and: condition.and, or: condition.or }"
          @click="switchBoolean()"
        >
          {{ condition.and ? 'AND' : 'OR' }}
        </v-chip>
      </div>

      <div class="actions">
        <v-btn color="promoPayBlue" size="small" class="mr-4">
          Add Expression
          <v-menu activator="parent">
            <v-list>
              <v-list-item title="Group" @click="addCondition('and')" />
              <v-list-item title="IP Address" @click="addCondition('ip')" />
              <v-list-item title="CIDR" @click="addCondition('cidr')" />
              <v-list-item title="Header" @click="addCondition('headerMatch')" />
              <v-list-item title="Header Regex" @click="addCondition('headerRegex')" />
            </v-list>
          </v-menu>
        </v-btn>
      </div>
    </div>

    <div class="wrapper">
      <v-chip class="parenthesis">)</v-chip>

      <v-chip v-if="!isTopLevel" variant="elevated" class="delete">
        <v-icon @click="emit('delete')">mdi-close</v-icon>
      </v-chip>
    </div>
  </div>
</template>

<style scoped>
.condition-group {
  margin: 10px 0;
}

.parenthesis {
  margin: 0 5px;
  background-color: #bbdefb;
}

.condition-not {
  opacity: 0.3;
  text-decoration: line-through;
  background-color: #c62828;
}

.condition-not.active {
  opacity: 1;
  text-decoration: none;
}

.sub-conditions {
  display: flex;
  flex-direction: column;
}

.sub-conditions > div {
  margin-left: 20px;
}

.actions {
  display: flex;
  align-items: center;
  margin: 5px 0 10px 0;
}

.boolean.and {
  background-color: #009688;
}

.boolean.or {
  background-color: #00bcd4;
}

.delete {
  cursor: pointer;
  background-color: #f44336;
  color: white;
  opacity: 0;
}

.wrapper:hover .delete {
  opacity: 1;
}
</style>
