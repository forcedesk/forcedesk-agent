<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Models\EdupassAccounts;
use App\Models\EdupassCrtAccounts;
use GuzzleHttp\Client;
use GuzzleHttp\Exception\GuzzleException;
use Illuminate\Console\Command;

class CrtAccountsService extends Command
{
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:crtaccount-service';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Performs a synchronization of CRT account data from the Edustar Management Console.';

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

        if (empty(config('agentconfig.emc.emc_username')) || empty(config('agentconfig.emc.emc_password')) || empty(config('agentconfig.emc.emc_school_code'))) {
            return false;
        }

        // The URL of the EduSTAR MC API endpoint.
        $url = 'https://apps.edustar.vic.edu.au/edustarmc/api/MC/GetGroupMembers?schoolId='.config('agentconfig.emc.emc_school_code').'&groupDn='.config('agentconfig.emc.emc_crt_group_dn').'&'.'&groupName='.config('agentconfig.emc.emc_crt_group_name');

        try {
            $client = new Client();
            $response = $client->get($url, [
                'auth' => [
                    config('agentconfig.emc.emc_username'),
                    config('agentconfig.emc.emc_password'),
                ],
            ]);

            foreach (json_decode($response->getbody()) as $item) {

                $crtaccount = EdupassCrtAccounts::where('login', $item->_login)->first();

                $data = [
                    'login' => $item->_login,
                    'displayName' => $item->_cn,
                    'daily_password' => 'Not Yet Set',
                    'ldap_dn' => $item->_dn,
                ];

                if($crtaccount)
                {
                    $crtaccount->displayName = $data['displayName'];
                    $crtaccount->ldap_dn = $data['ldap_dn'];
                    $crtaccount->save();
                } else {
                    $crtaccount = new EdupassCrtAccounts;
                    $crtaccount->login = $data['login'];
                    $crtaccount->displayName = $data['displayName'];
                    $crtaccount->daily_password = $data['daily_password'];
                    $crtaccount->ldap_dn = $data['ldap_dn'];
                    $crtaccount->save();
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
        $del_accounts = EduPassCrtAccounts::whereNotIn('login', $accounts)->get();

        foreach($del_accounts as $del_account)
        {
            $del_account->delete();
            $deletecount++;
        }
        /* End Delete Old Accounts */

        try {

            $sdclient = new Client(['verify' => false, 'headers' => array(
                'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                'Content-Type' => 'application/json',
                'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                'x-forcedesk-agentversion' => config('app.agent_version'),
            )]);

            $this->info('Posting Data to '.config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/crtaccount-data');

            $response = $sdclient->post(config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/crtaccount-data', [
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
