<?php

use Illuminate\Support\Facades\Route;
use App\Http\Controllers\AgentSettingsController;
use App\Http\Controllers\AuthController;

/*
|--------------------------------------------------------------------------
| Web Routes
|--------------------------------------------------------------------------
|
| Here is where you can register web routes for your application. These
| routes are loaded by the RouteServiceProvider within a group which
| contains the "web" middleware group. Now create something great!
|
*/

// Authentication routes
Route::get('/login', [AuthController::class, 'showLogin'])->name('agent.login');
Route::post('/login', [AuthController::class, 'login'])->name('agent.login.post');
Route::post('/logout', [AuthController::class, 'logout'])->name('agent.logout');

// Protected routes - require authentication
Route::middleware('agent.auth')->group(function () {
    Route::get('/agent-settings', [AgentSettingsController::class, 'index'])->name('agent-settings.index');
    Route::get('/agent-settings/all', [AgentSettingsController::class, 'getAll'])->name('agent-settings.all');
    Route::put('/agent-settings', [AgentSettingsController::class, 'update'])->name('agent-settings.update');
    Route::put('/agent-settings/{id}', [AgentSettingsController::class, 'updateSingle'])->name('agent-settings.update-single');
    Route::post('/agent-settings/import', [AgentSettingsController::class, 'importFromConfig'])->name('agent-settings.import');
    Route::get('/agent-settings/export', [AgentSettingsController::class, 'exportConfig'])->name('agent-settings.export');
    Route::post('/agent-settings/clear-cache', [AgentSettingsController::class, 'clearCache'])->name('agent-settings.clear-cache');
    Route::post('/agent-settings/test-connection', [AgentSettingsController::class, 'testConnection'])->name('agent-settings.test-connection');
});