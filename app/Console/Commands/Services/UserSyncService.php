<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use Illuminate\Console\Command;

class UserSyncService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:usersync-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Updates the local user database with the remote source.';

    public function handle()
    {

        $test = AgentConnectivityHelper::testLdapConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        // Call Artisan to import Staff via the defined LDAP Scope.
        \Artisan::call('ldap:import staff --filter="(memberof='.config('agentconfig.ldap.staff_scope').')" -n --delete --restore --delete-missing --no-log --quiet');

        // Call Artisan to import Students via the defined LDAP Scope.
        \Artisan::call('ldap:import students --filter="(memberof='.config('agentconfig.ldap.student_scope').')" -n --delete --restore --delete-missing --no-log --quiet');

        return true;

    }
}
