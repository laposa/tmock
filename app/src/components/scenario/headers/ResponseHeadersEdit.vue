<script setup lang="ts">
import type { ResponseHeaderItem } from './ResponseHeaderItem.vue';

const emit = defineEmits<{
	'update:model-value': [ResponseHeaderItem[]],
}>();	

const headers = defineModel<ResponseHeaderItem[]>({ required: true });

function updateHeader(index: number, { header, value }: ResponseHeaderItem) {
	headers.value[index] = { header, value };

	emit('update:model-value', headers.value);
}

function removeHeader(index: number) {
	headers.value.splice(index, 1);

	emit('update:model-value', headers.value);
}

function addHeader() {
	headers.value.push({ header: '', value: '' });

	emit('update:model-value', headers.value);
}
</script>

<template>
	<div class="headers">
		<div class="row" v-for="(header, index) in headers" :key="index">
			<ResponseHeaderItem 
				:model-value="header"
				@update:model-value="(val) => updateHeader(index, val)"
				@delete="removeHeader(index)"
			/>
		</div>
		<v-btn 
			prepend-icon="mdi-plus"
			@click="addHeader"> 
				Add header
		</v-btn>
	</div>
</template>

<style scoped>
.headers {
	padding-left: 55px;

	.row:last-of-type {
		margin-bottom: 10px;
	}
}
</style>
