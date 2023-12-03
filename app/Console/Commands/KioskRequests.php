<?php

namespace App\Console\Commands;

use App\Services\PasswordResetService;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class KioskRequests extends Command
{
    protected PasswordResetService $passwordResetService;

    public function __construct(PasswordResetService $passwordResetService)
    {
        parent::__construct();
        $this->passwordResetService = $passwordResetService;
    }
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:kiosk-requests';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Run the agent and checks for kiosk requests.';

    /**
     * Execute the console command.
     *
     * @return int
     */
    public function handle()
    {

        if(config('agentconfig.agent.strict_logging') == true) {
            \Log::info('Logging is enabled');
        }

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/kiosk/payloads');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        if (count($data) == '0')
        {
            $this->error('No kiosk payloads received');
            return false;
        }

        foreach ($data as $item) {

            $response = $this->passwordResetService->handlePasswordReset($item->payload_data->kioskid, $item->payload_data->username);

            $payload = [
                'kioskid' => $item->payload_data->kioskid,
                'resetdata' => $response,
            ];

            try {
                $this->info('Posting Data to '.config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/passwordreset');

                $response = $sdclient->post(config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/passwordreset', [
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

            } catch (\Exception $e)
            {
                \Log::error('Could not send data to SchoolDesk Tenant');
            }

        }

    }
}
