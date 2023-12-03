<?php

namespace App\Console\Commands\SchoolDesk;

use App\Models\EdupassAccounts;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class EdustarService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:edustar-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Performs a synchronization of student data from EdustarMC.';

    /**
     * Create a new command instance.
     *
     * @return void
     */
    public function __construct()
    {
        parent::__construct();
    }

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        $importcount = 0;

        if (empty(config('agentconfig.emc.emc_username')) || empty(config('agentconfig.emc.emc_password')) || empty(config('agentconfig.emc.emc_school_code'))) {
            return false;
        }

        // The URL of the EduSTAR MC API endpoint.
        $url = 'https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetStudents/'.config('agentconfig.emc.emc_school_code').'/FULL';

        try {
            $client = new Client();
            $response = $client->get($url, [
                'auth' => [
                    config('agentconfig.emc.emc_username'),
                    config('agentconfig.emc.emc_password'),
                ],
            ]);

            foreach (json_decode($response->getbody()) as $item) {

                $edupassaccount = EdupassAccounts::where('login', $item->_login)->first();

                if (! $edupassaccount) {
                    EdupassAccounts::create(
                        [
                            'login' => $item->_login,
                            'firstName' => $item->_firstName,
                            'lastName' => $item->_lastName,
                            'password' => 'Not Yet Set',
                            'displayName' => $item->_displayName,
                            'ldap_dn' => $item->_dn,
                        ]);
                    $importcount++;
                }

            }
        } catch (\Exception $e) {
            $this->warn($e->getMessage());

            return false;
        }

        $this->info('Imported '.$importcount.' accounts.');

    }
}
