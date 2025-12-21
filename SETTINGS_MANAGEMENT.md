# Agent Settings Management System

This document describes the database-backed settings management system for the ForceDesk Agent.

## Overview

The agent configuration has been migrated from a static `config/agentconfig.php` file to a database-backed system with a Vue.js/Inertia.js interface. Settings can now be managed through a web interface and are cached for performance.

## Components Created

### Backend Components

1. **Database Migration**: `database/migrations/2025_12_21_101113_create_agent_settings_table.php`
   - Creates the `agent_settings` table with support for different data types
   - Includes sensitive field marking for passwords/API keys
   - Groups settings by category (tenant, ldap, emc, etc.)

2. **Model**: `app/Models/AgentSetting.php`
   - Handles CRUD operations for settings
   - Includes caching for performance
   - Type casting for boolean, integer, float, and JSON values

3. **Helper Service**: `app/Helpers/AgentConfig.php`
   - Provides database-first lookup with config file fallback
   - Import functionality from config file
   - Type detection and sensitive field identification

4. **Helper Function**: `app/helpers.php`
   - Global `agent_config()` function for easy access
   - Usage: `agent_config('tenant.tenant_url')`

5. **Controller**: `app/Http/Controllers/AgentSettingsController.php`
   - Web interface endpoints
   - Settings update API
   - Import from config functionality
   - Cache management

6. **Seeder**: `database/seeders/AgentConfigSeeder.php`
   - Seeds database from config file

### Frontend Components

1. **Layout**: `resources/js/Layouts/AppLayout.vue`
   - Main application layout

2. **Settings Page**: `resources/js/Pages/AgentConfig/Index.vue`
   - Complete settings management interface
   - Grouped by category
   - Sensitive field masking
   - Bulk save functionality
   - Import from config button

3. **Inertia Setup**:
   - Vue 3 and Inertia.js configured
   - Tailwind CSS for styling
   - Vite build system

## Setup Instructions

### 1. Install Dependencies

```bash
composer dump-autoload
npm install
```

### 2. Run Migrations

```bash
php artisan migrate
```

### 3. Import Initial Settings

You can import settings from the config file in two ways:

**Option A: Via Seeder**
```bash
php artisan db:seed --class=AgentConfigSeeder
```

**Option B: Via Web Interface**
- Navigate to `/agent-settings`
- Click "Import from Config" button

### 4. Build Frontend Assets

```bash
npm run dev    # For development
npm run build  # For production
```

### 5. Access the Settings Interface

Navigate to: `http://your-domain/agent-settings`

## Usage

### Accessing Settings in Code

All previous `config('agentconfig.*')` calls have been replaced with `agent_config()`:

```php
// Old way
$apiKey = config('agentconfig.tenant.tenant_api_key');

// New way
$apiKey = agent_config('tenant.tenant_api_key');

// With default value
$enabled = agent_config('logging.enabled', false);
```

### Setting Values Programmatically

```php
use App\Helpers\AgentConfig;

// Set a single value
AgentConfig::set('tenant.tenant_url', 'https://example.com', 'string', false);

// Set a sensitive value
AgentConfig::set('tenant.tenant_api_key', 'secret-key', 'string', true);

// Clear cache after bulk updates
AgentConfig::clearCache();
```

### Routes

- `GET /agent-settings` - Settings management interface
- `GET /agent-settings/all` - Get all settings (JSON)
- `PUT /agent-settings` - Update multiple settings
- `PUT /agent-settings/{id}` - Update single setting
- `POST /agent-settings/import` - Import from config file
- `POST /agent-settings/clear-cache` - Clear settings cache

## Settings Structure

Settings are organized into groups matching the original config file:

- **tenant**: Tenant connection settings (URL, API key, UUID)
- **device_manager**: Device manager configuration
- **logging**: Logging settings
- **papercut**: PaperCut integration settings
- **proxies**: Proxy configuration
- **emc**: EMC/Edustar settings
- **ldap**: LDAP/Active Directory settings

## Security Features

1. **Sensitive Field Masking**: Password and API key fields are masked in the UI
2. **Type Safety**: Values are type-cast based on their declared type
3. **Caching**: Settings are cached for 1 hour to reduce database queries
4. **Fallback**: If database value doesn't exist, falls back to config file

## Caching

Settings are cached for performance. The cache is automatically cleared when:
- Settings are updated via the web interface
- Settings are imported from config
- `AgentConfig::clearCache()` is called manually

Cache key format: `agent_setting_{key}`

## Migration Path

All references to `config('agentconfig.*')` have been automatically updated to use `agent_config()`. This provides:

1. Database-first lookup
2. Automatic fallback to config file if DB value not found
3. Runtime updateable settings without code deployment
4. Better audit trail of setting changes

## Future Enhancements

Potential improvements:
- Setting history/audit log
- Connection testing for tenant/LDAP settings
- Setting validation
- Export settings to config file
- User access controls
- Setting descriptions in the UI
