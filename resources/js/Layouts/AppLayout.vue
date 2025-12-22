<template>
    <div class="min-h-screen bg-gray-100 flex flex-col">
        <nav class="bg-white shadow">
            <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
                <div class="flex justify-between h-16">
                    <div class="flex items-center gap-3">
                        <img
                            src="https://cdn.forcedesk.io/img/forcedesk-icon-test.svg"
                            alt="ForceDesk Logo"
                            class="h-8 w-8"
                        />
                        <h1 class="text-xl font-semibold text-gray-900">
                            Agent Configuration
                        </h1>
                    </div>
                    <div class="flex items-center">
                        <button
                            @click="handleLogout"
                            class="inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-gray-700 bg-gray-100 hover:bg-gray-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500"
                        >
                            <LogOut class="h-4 w-4 mr-2" />
                            Logout
                        </button>
                    </div>
                </div>
            </div>
        </nav>

        <main class="flex-1 py-10">
            <div class="max-w-7xl mx-auto sm:px-6 lg:px-8">
                <slot />
            </div>
        </main>

        <footer class="bg-white border-t border-gray-200 mt-auto">
            <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-4">
                <p class="text-center text-sm text-gray-500">
                    &copy; {{ new Date().getFullYear() }} ForceDesk. All rights reserved.
                </p>
            </div>
        </footer>

        <!-- Logout Confirmation Dialog -->
        <ConfirmDialog
            :show="showLogoutDialog"
            title="Confirm Logout"
            message="Are you sure you want to logout?"
            confirmText="Logout"
            cancelText="Cancel"
            type="danger"
            @confirm="confirmLogout"
            @cancel="showLogoutDialog = false"
        />
    </div>
</template>

<script setup>
import { ref } from 'vue';
import { router } from '@inertiajs/vue3';
import { LogOut } from 'lucide-vue-next';
import ConfirmDialog from '@/Components/ConfirmDialog.vue';

const showLogoutDialog = ref(false);

function handleLogout() {
    showLogoutDialog.value = true;
}

function confirmLogout() {
    showLogoutDialog.value = false;
    router.post('/logout');
}
</script>
