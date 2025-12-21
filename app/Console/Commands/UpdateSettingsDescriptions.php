<?php

namespace App\Console\Commands;

use App\Models\AgentSetting;
use Illuminate\Console\Command;

class UpdateSettingsDescriptions extends Command
{
    protected $signature = 'agent:update-descriptions';
    protected $description = 'Update existing settings with descriptive labels';

    public function handle()
    {
        $descriptions = [
            // Tenant settings
            'tenant.tenant_url' => 'The URL of your ForceDesk tenant (e.g., https://your-tenant.forcedesk.io)',
            'tenant.tenant_api_key' => 'API key for authenticating with the ForceDesk tenant',
            'tenant.tenant_uuid' => 'Unique identifier for this agent instance',
            'tenant.verify_ssl' => 'Verify SSL certificates when connecting to the tenant',

            // Device Manager settings
            'device_manager.legacycommand' => 'Legacy SSH command options for older Cisco devices',

            // Logging settings
            'logging.enabled' => 'Enable or disable detailed logging for this agent',

            // PaperCut settings
            'papercut.api_url' => 'PaperCut server API URL (e.g., http://papercut-server:9191/api)',
            'papercut.api_key' => 'Authentication key for PaperCut API access',
            'papercut.enabled' => 'Enable or disable PaperCut integration',

            // Proxy settings
            'proxies.address' => 'HTTP/HTTPS proxy server address (e.g., http://proxy.example.com:8080)',

            // EMC/Edustar settings
            'emc.emc_url' => 'Edustar Management Console API endpoint URL',
            'emc.emc_username' => 'Username for Edustar MC authentication',
            'emc.emc_password' => 'Password for Edustar MC authentication',
            'emc.emc_school_code' => 'Your school code in the Edustar system',
            'emc.emc_crt_group_dn' => 'Distinguished Name of the CRT group in Active Directory',
            'emc.emc_crt_group_name' => 'Name of the CRT group for student accounts',

            // LDAP settings
            'ldap.ad_dc' => 'Primary Active Directory domain controller hostname or IP',
            'ldap.ad_svc_user_cn' => 'Service account username (CN) for LDAP operations',
            'ldap.ad_svc_password' => 'Service account password for LDAP authentication',
            'ldap.ad_base_dn' => 'Base Distinguished Name for LDAP searches (e.g., DC=school,DC=local)',
            'ldap.staff_scope' => 'LDAP group DN containing staff members',
            'ldap.student_scope' => 'LDAP group DN containing student accounts',
        ];

        $updated = 0;

        foreach ($descriptions as $key => $description) {
            $setting = AgentSetting::where('key', $key)->first();
            if ($setting) {
                $setting->update(['description' => $description]);
                $updated++;
                $this->info("Updated: {$key}");
            }
        }

        $this->info("\nUpdated {$updated} settings with descriptions.");

        return 0;
    }
}
