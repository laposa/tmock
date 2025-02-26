<script setup lang="ts">

const emit = defineEmits<{
  'delete': [ResponseHeaderItem];
}>();

export type ResponseHeaderItem = {
	header: string,
	value: string,
};

const props = defineProps<{
	header: ResponseHeaderItem,
}>();

const header = ref(props.header);

</script>

<template>
	<div class="item">
		<v-text-field 
			v-model="header.header" 
			density="compact"
			:hide-details="true">
		</v-text-field>: 
		
		<v-text-field 
			v-model="header.value" 
			density="compact"
			:hide-details="true">
		</v-text-field>
		
		<v-btn 
			icon="mdi-close" 
			size="x-small"
			color="red"
			@click="emit('delete', header)">
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
