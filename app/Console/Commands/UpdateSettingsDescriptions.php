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
        $settings = [
            // Tenant settings
            'tenant.tenant_url' => [
                'label' => 'Tenant URL',
                'description' => 'The URL of your ForceDesk tenant (e.g., https://your-tenant.forcedesk.io)',
            ],
            'tenant.tenant_api_key' => [
                'label' => 'API Key',
                'description' => 'API key for authenticating with the ForceDesk tenant',
            ],
            'tenant.tenant_uuid' => [
                'label' => 'Agent UUID',
                'description' => 'Unique identifier for this agent instance',
            ],
            'tenant.verify_ssl' => [
                'label' => 'Verify SSL',
                'description' => 'Verify SSL certificates when connecting to the tenant',
            ],

            // Device Manager settings
            'device_manager.legacycommand' => [
                'label' => 'Legacy Command',
                'description' => 'Legacy SSH command options for older Cisco devices',
            ],

            // Logging settings
            'logging.enabled' => [
                'label' => 'Enable Logging',
                'description' => 'Enable or disable detailed logging for this agent',
            ],

            // PaperCut settings
            'papercut.api_url' => [
                'label' => 'PaperCut Server URL',
                'description' => 'Your PaperCut server API URL (e.g., http://papercut-server:9191/api)',
            ],
            'papercut.api_key' => [
                'label' => 'PaperCut API Key',
                'description' => 'Authentication key for PaperCut API access',
            ],
            'papercut.enabled' => [
                'label' => 'Enable PaperCut',
                'description' => 'Enable or disable PaperCut integration',
            ],

            // Proxy settings
            'proxies.address' => [
                'label' => 'Proxy Address',
                'description' => 'HTTP/HTTPS proxy server address (e.g., http://proxy.example.com:8080)',
            ],

            // EMC/Edustar settings
            'emc.emc_url' => [
                'label' => 'EduSTAR MC URL',
                'description' => 'Edustar Management Console API endpoint URL',
            ],
            'emc.emc_username' => [
                'label' => 'Username',
                'description' => 'Username for Edustar MC authentication',
            ],
            'emc.emc_password' => [
                'label' => 'Password',
                'description' => 'Password for Edustar MC authentication',
            ],
            'emc.emc_school_code' => [
                'label' => 'School Code',
                'description' => 'Your school code in the Edustar system',
            ],
            'emc.emc_crt_group_dn' => [
                'label' => 'CRT Group DN',
                'description' => 'Distinguished Name of the CRT group in Active Directory',
            ],
            'emc.emc_crt_group_name' => [
                'label' => 'CRT Group Name',
                'description' => 'Name of the CRT group for student accounts',
            ],

            // LDAP settings
            'ldap.ad_dc' => [
                'label' => 'Domain Controller',
                'description' => 'Primary Active Directory domain controller hostname or IP',
            ],
            'ldap.ad_svc_user_cn' => [
                'label' => 'Service Account Username',
                'description' => 'Service account username (CN) for LDAP operations',
            ],
            'ldap.ad_svc_password' => [
                'label' => 'Service Account Password',
                'description' => 'Service account password for LDAP authentication',
            ],
            'ldap.ad_base_dn' => [
                'label' => 'Base DN',
                'description' => 'Base Distinguished Name for LDAP searches (e.g., DC=school,DC=local)',
            ],
            'ldap.staff_scope' => [
                'label' => 'Staff Group DN',
                'description' => 'LDAP group DN containing staff members',
            ],
            'ldap.student_scope' => [
                'label' => 'Student Group DN',
                'description' => 'LDAP group DN containing student accounts',
            ],
        ];

        $updated = 0;

        foreach ($settings as $key => $data) {
            $setting = AgentSetting::where('key', $key)->first();
            if ($setting) {
                $setting->update([
                    'label' => $data['label'],
                    'description' => $data['description'],
                ]);
                $updated++;
                $this->info("Updated: {$key}");
            }
        }

        $this->info("\nUpdated {$updated} settings with labels and descriptions.");

        return 0;
    }
}
