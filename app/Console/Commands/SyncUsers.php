<?php

namespace App\Console\Commands;

use App\Models\User;
use Illuminate\Console\Command;

class SyncUsers extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:syncusers';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Updates the local user database with the remote source.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {
        // Call Artisan to import Staff via the defined LDAP Scope.
        \Artisan::call('ldap:import staff --filter="(memberof='.config('agentconfig.ldap.staff_scope').')" -n --delete --restore --delete-missing --no-log --quiet');

        // Call Artisan to import Students via the defined LDAP Scope.
        \Artisan::call('ldap:import students --filter="(memberof='.config('agentconfig.ldap.student_scope').')" -n --delete --restore --delete-missing --no-log --quiet');

    }
}
