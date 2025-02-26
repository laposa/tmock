<script setup lang="ts">

const emit = defineEmits<{
	'delete': [];
	'update:model-value': [ResponseHeaderItem];
}>();

export type ResponseHeaderItem = {
	header: string,
	value: string,
};

const header = defineModel<ResponseHeaderItem>({ required: true });

function updateHeader(header: string, value: string) {
	emit('update:model-value', { header, value });
}
</script>

<template>
	<div class="item">
		<v-text-field 
			:model-value="header.header"
			@update:model-value="(val) => updateHeader(val, header.value)"
			density="compact"
			:hide-details="true">
		</v-text-field>: 
		
		<v-text-field 
			:model-value="header.value"
			@update:model-value="(val) => updateHeader(header.header, val)"
			density="compact"
			:hide-details="true">
		</v-text-field>
		
		<v-btn 
			icon="mdi-close" 
			size="x-small"
			color="red"
			@click="emit('delete')">
		</v-btn>
	</div>
</template>

<style scoped>
.item {
	display: flex;
	width: 100%;
	gap: 10px;
	align-items: center;
}
</style>
