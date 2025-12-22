<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Models\EdupassAccounts;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;
use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;

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
    protected $description = 'Performs a synchronization of student data from the Edustar Management Console.';

    /**
     * Create a new command instance.
     *
     * @return void
     */
    public function __construct()
    {
        parent::__construct();
    }

    private function generateRandomWord(): string
    {
        $phrases = ['wild', 'wood', 'frog', 'beast', 'lion', 'zebra', 'marmoset', 'unicorn', 'rabbit', 'bear', 'float', 'berry', 'puppy', 'cat', 'horse', 'river', 'ocean', 'skies', 'sky', 'golden', 'pear', 'apple', 'raspberry', 'watermelon', 'kiwi', 'basket', 'soccer', 'football', 'jazz', 'fair', 'field', 'insect', 'stick', 'jump', 'bridge', 'log', 'abacus', 'tinsel', 'coat', 'door', 'window', 'free', 'peach', 'cress', 'creek', 'croak', 'crest', 'dino', 'rascal', 'clifford', 'franklin', 'zelda', 'link', 'mario', 'bowser'];

        return $phrases[array_rand($phrases)];
    }

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        $importcount = 0;
        $deletecount = 0;
        $accounts = [];
        $payload = [];

        if (empty(agent_config('emc.emc_username')) || empty(agent_config('emc.emc_password')) || empty(agent_config('emc.emc_school_code'))) {
            return false;
        }

        // The URL of the EduSTAR MC API endpoint.
        $url = 'https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetStudents/'.agent_config('emc.emc_school_code').'/FULL';

        try {
            $client = new Client();
            $response = $client->get($url, [
                'auth' => [
                    agent_config('emc.emc_username'),
                    agent_config('emc.emc_password'),
                ],
            ]);

            foreach (json_decode($response->getbody()) as $item) {

                $edupassaccount = EdupassAccounts::where('login', $item->_login)->first();

                /* We also want to set a password for the account */
                $genpassword = ucwords($this->generateRandomWord()).'.'.rand(1000, 9999);

                $data = [
                    'login' => $item->_login,
                    'firstName' => $item->_firstName,
                    'lastName' => $item->_lastName,
                    'password' => $genpassword,
                    'displayName' => $item->_displayName,
                    'student_class' => $item->_class,
                    'ldap_dn' => $item->_dn,
                ];

                if($edupassaccount)
                {
                    $edupassaccount->firstName = $data['firstName'];
                    $edupassaccount->lastName = $data['lastName'];
                    $edupassaccount->displayName = $data['displayName'];
                    $edupassaccount->student_class = $data['student_class'];
                    $edupassaccount->ldap_dn = $data['ldap_dn'];
                    $edupassaccount->save();
                } else {

                    try {
                        Http::withBasicAuth(agent_config('emc.emc_username'), agent_config('emc.emc_password'))->retry(5, 100)->post('https://apps.edustar.vic.edu.au/edustarmc/api/MC/ResetStudentPwd', ['schoolId' => agent_config('emc.emc_school_code'), 'dn' => $data['ldap_dn'], 'newPass' => $genpassword]);
                    } catch (ConnectionException) {
                        \Log::error('Could not reset student password for '.$item->_login);
                    }

                    $edupassaccount = new EdupassAccounts;
                    $edupassaccount->login = $data['login'];
                    $edupassaccount->firstName = $data['firstName'];
                    $edupassaccount->lastName = $data['lastName'];
                    $edupassaccount->displayName = $data['displayName'];
                    $edupassaccount->password = $genpassword;
                    $edupassaccount->student_class = $data['student_class'];
                    $edupassaccount->ldap_dn = $data['ldap_dn'];
                    $edupassaccount->save();
                }

                $accounts[] = $data['login'];

                $payload[] = $data;

                $importcount++;

            }
        } catch (\Exception $e) {
            $this->warn($e->getMessage());

            return false;
        }

        /* Delete Accounts that have been removed */
        $del_accounts = \App\Models\EduPassAccounts::whereNotIn('login', $accounts)->get();

        foreach($del_accounts as $del_account)
        {
            $del_account->delete();
            $deletecount++;
        }
        /* End Delete Old Accounts */

        try {

            $sdclient = new Client(['verify' => false, 'headers' => array(
                'Authorization' => 'Bearer ' . agent_config('tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => agent_config('tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
            )]);

            $this->info('Posting Data to '.agent_config('tenant.tenant_url') . '/api/agent/ingest/edustar-data');

            $response = $sdclient->post(agent_config('tenant.tenant_url') . '/api/agent/ingest/edustar-data', [
                'headers' => [],
                'body' => json_encode($payload),
            ]);

            $status = json_decode($response->getBody(), false);

            if($status->status != 'ok')
            {
                $this->error('There was an error sending the data to SchoolDesk!');
                $this->error($status->message);
            } else {
                $this->info('Posting the data succeeded!');
                $this->info($status->message);
            }

        } catch (GuzzleException $e)
        {
            $this->warn($e->getMessage());

            return false;
        }

        $this->info('Deleted '.$deletecount.' accounts.');

        return true;

    }
}
