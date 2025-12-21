<template>
    <AppLayout>
        <div class="bg-white shadow overflow-hidden sm:rounded-lg">
            <div class="px-4 py-5 sm:px-6 flex justify-between items-center">
                <div>
                    <h3 class="text-lg leading-6 font-medium text-gray-900">
                        Agent Configuration Settings
                    </h3>
                    <p class="mt-1 max-w-2xl text-sm text-gray-500">
                        Manage all agent configuration settings from the database.
                    </p>
                </div>
                <div class="flex gap-2">
                    <button
                        @click="importFromConfig"
                        class="inline-flex items-center px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                    >
                        Import from Config
                    </button>
                    <a
                        href="/agent-settings/export"
                        class="inline-flex items-center px-4 py-2 border border-gray-300 rounded-md shadow-sm text-sm font-medium text-gray-700 bg-white hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                    >
                        Export Config File
                    </a>
                    <button
                        @click="saveAllSettings"
                        :disabled="saving"
                        class="inline-flex items-center px-4 py-2 border border-transparent rounded-md shadow-sm text-sm font-medium text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
                    >
                        {{ saving ? 'Saving...' : 'Save All Changes' }}
                    </button>
                </div>
            </div>

            <div class="border-t border-gray-200">
                <div v-if="message" class="px-4 py-3 bg-green-50 border-b border-green-200">
                    <p class="text-sm text-green-800">{{ message }}</p>
                </div>

                <div v-if="error" class="px-4 py-3 bg-red-50 border-b border-red-200">
                    <p class="text-sm text-red-800">{{ error }}</p>
                </div>

                <div class="px-4 py-5 sm:p-6">
                    <div v-for="(groupSettings, groupName) in localSettings" :key="groupName" class="mb-8">
                        <h4 class="text-md font-semibold text-gray-900 mb-4 capitalize border-b pb-2">
                            {{ groupName }} Settings
                        </h4>
                        <div class="space-y-4">
                            <div
                                v-for="setting in groupSettings"
                                :key="setting.id"
                                class="grid grid-cols-1 gap-4 sm:grid-cols-3"
                            >
                                <div class="sm:col-span-1">
                                    <label :for="`setting-${setting.id}`" class="block text-sm font-medium text-gray-700">
                                        {{ formatKey(setting.key) }}
                                        <span v-if="setting.is_sensitive" class="text-red-500">*</span>
                                    </label>
                                    <p v-if="setting.description" class="mt-1 text-xs text-gray-500">
                                        {{ setting.description }}
                                    </p>
                                </div>
                                <div class="sm:col-span-2">
                                    <input
                                        v-if="setting.type === 'boolean'"
                                        :id="`setting-${setting.id}`"
                                        type="checkbox"
                                        v-model="setting.actual_value"
                                        class="h-4 w-4 text-indigo-600 focus:ring-indigo-500 border-gray-300 rounded"
                                    />
                                    <input
                                        v-else
                                        :id="`setting-${setting.id}`"
                                        :type="setting.is_sensitive ? 'password' : 'text'"
                                        v-model="setting.actual_value"
                                        :placeholder="setting.is_sensitive ? '********' : 'Enter value'"
                                        class="shadow-sm focus:ring-indigo-500 focus:border-indigo-500 block w-full sm:text-sm border-gray-300 rounded-md"
                                    />
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </AppLayout>
</template>

<script setup>
import { ref, reactive } from 'vue';
import { router } from '@inertiajs/vue3';
import AppLayout from '@/Layouts/AppLayout.vue';

const props = defineProps({
    settings: Object,
    groups: Array,
});

const localSettings = reactive({ ...props.settings });
const saving = ref(false);
const message = ref('');
const error = ref('');

function formatKey(key) {
    const parts = key.split('.');
    const lastPart = parts[parts.length - 1];
    return lastPart.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
}

async function saveAllSettings() {
    saving.value = true;
    message.value = '';
    error.value = '';

    const settingsToUpdate = [];
    Object.values(localSettings).forEach(group => {
        group.forEach(setting => {
            settingsToUpdate.push({
                id: setting.id,
                value: setting.actual_value,
            });
        });
    });

    try {
        const response = await fetch('/agent-settings', {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
            body: JSON.stringify({ settings: settingsToUpdate }),
        });

        const data = await response.json();

        if (response.ok) {
            message.value = 'Settings saved successfully!';
            setTimeout(() => {
                message.value = '';
            }, 3000);
        } else {
            error.value = data.message || 'Failed to save settings';
        }
    } catch (err) {
        error.value = 'An error occurred while saving settings';
    } finally {
        saving.value = false;
    }
}

async function importFromConfig() {
    try {
        const response = await fetch('/agent-settings/import', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]')?.content,
            },
        });

        const data = await response.json();

        if (response.ok) {
            message.value = data.message;
            setTimeout(() => {
                router.reload();
            }, 1000);
        } else {
            error.value = data.message || 'Failed to import settings';
        }
    } catch (err) {
        error.value = 'An error occurred while importing settings';
    }
}
</script>
