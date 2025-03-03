<?php

namespace App\Console\Commands\Services;

use App\Helper\AgentConnectivityHelper;
use App\Models\Students;
use App\Models\User;
use App\Services\PasswordResetService;
use GuzzleHttp\Client;
use Illuminate\Console\Command;

class CommandQueueService extends Command
{
    public function __construct()
    {
        parent::__construct();
    }
    /**
     * The name and signature of the console command.
     *
     * @var string
     */
    protected $signature = 'agent:process-command-queue';

    /**
     * The console command description.
     *
     * @var string
     */
    protected $description = 'Run the agent and check for any command queues.';

    public function handle()
    {

        $test = AgentConnectivityHelper::testConnectivity();

        if(!$test)
        {
            \Log::error('Could not connect to the SchoolDesk instance.');
            $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
            return false;
        }

        $client = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-forcedesk-agentversion' => config('app.agent_version'),
        )]);

        $request = $client->get(config('agentconfig.tenant.tenant_url') . '/api/agent/command-queues');

        $response = $request->getBody()->getContents();
        $data = json_decode($response, false);

        foreach($data as $item)
        {

            if($item->type == 'force-sync-edustarsvc' && $item->payload_data->process)
            {
                /* Fire the console command */
                \Artisan::call('agent:edustar-service');
            }

            if($item->type == 'force-sync-papercutsvc' && $item->payload_data->process)
            {
                $test = AgentConnectivityHelper::testConnectivity();

                if(!$test)
                {
                    \Log::error('Could not connect to the SchoolDesk instance.');
                    $this->error('Connectivity failed to the SchoolDesk instance. Bailing out');
                    return false;
                }

                $sdclient = new Client(['verify' => false, 'headers' => array(
                    'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
                    'Content-Type' => 'application/json',
                    'x-forcedesk-agent' => config('agentconfig.tenant.tenant_uuid'),
                    'x-forcedesk-agentversion' => config('app.agent_version'),
                )]);

                $api_key = config('agentconfig.papercut.api_key');
                $api_url = config('agentconfig.papercut.api_url');

                $students = Students::whereNotNull('username')->orderby('username','asc')->get();
                $staff = User::whereNotNull('staff_code')->orderby('staff_code','asc')->get();

                if (!$students && !$staff) {
                    $this->info('No staff or students found to process.');
                }

                $payload = [];

                foreach($staff as $object)
                {
                    try {
                        $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $object->staff_code . '</value>
                    </param>
                    <param>
                    <value>secondary-card-number</value>
                    </param>
                </params>
            </methodCall>
            ';

                        $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                        $client = new Client();

                        $response = $client->request('POST', $api_url, $options);

                        $xmlObject = simplexml_load_string($response->getbody());

                        $jsonFormatData = json_encode($xmlObject);
                        $result = json_decode($jsonFormatData, true);

                        $data['username'] = $object->staff_code;
                        $data['pin'] = $result['params']['param']['value'];

                        /* Grab the Balance */
                        $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $object->staff_code . '</value>
                    </param>
                    <param>
                    <value>balance</value>
                    </param>
                </params>
            </methodCall>
            ';

                        $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                        $client = new Client();

                        $response = $client->request('POST', $api_url, $options);

                        $xmlObject = simplexml_load_string($response->getbody());

                        $jsonFormatData = json_encode($xmlObject);
                        $result = json_decode($jsonFormatData, true);

                        $data['balance'] = $result['params']['param']['value'];

                        /* Push the Data to the array */
                        if(is_numeric($data['balance']) && is_numeric($data['pin']))
                        {
                            $this->info('Processed PIN and Balance for '.$object->staff_code);
                            $payload['staff'][] = $data;
                        }

                    } catch (\Exception $e)
                    {
                        \Log::error($e->getMessage());
                        \Log::error($e->getTraceAsString());
                        \Log::error('Could not get data for '.$object->staff_code);
                    }

                }

                foreach($students as $student)
                {
                    try {
                        $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $student->username . '</value>
                    </param>
                    <param>
                    <value>secondary-card-number</value>
                    </param>
                </params>
            </methodCall>
            ';

                        $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                        $client = new Client();

                        $response = $client->request('POST', $api_url, $options);

                        $xmlObject = simplexml_load_string($response->getbody());

                        $jsonFormatData = json_encode($xmlObject);
                        $result = json_decode($jsonFormatData, true);

                        $data['username'] = $student->username;
                        $data['pin'] = $result['params']['param']['value'];

                        /* Grab the Balance */
                        $xml = '<?xml version="1.0"?>
            <methodCall>
                <methodName>api.getUserProperty</methodName>
                <params>
                    <param>
                    <value>' . $api_key . '</value>
                    </param>
                    <param>
                    <value>' . $student->username . '</value>
                    </param>
                    <param>
                    <value>balance</value>
                    </param>
                </params>
            </methodCall>
            ';

                        $options = ['headers' => ['Content-Type' => 'text/xml; charset=UTF8',], 'body' => $xml,];

                        $client = new Client();

                        $response = $client->request('POST', $api_url, $options);

                        $xmlObject = simplexml_load_string($response->getbody());

                        $jsonFormatData = json_encode($xmlObject);
                        $result = json_decode($jsonFormatData, true);

                        $data['balance'] = $result['params']['param']['value'];

                        /* Push the Data to the array */

                        if(is_numeric($data['balance']) && is_numeric($data['pin']))
                        {
                            $this->info('Processed PIN and Balance for '.$student->username);
                            $payload['students'][] = $data;
                        }

                    } catch (\Exception $e)
                    {
                        \Log::error($e->getMessage());
                        \Log::error($e->getTraceAsString());
                        \Log::error('Could not get Papercut Data for '.$student->username);
                    }

                }

                try {

                    $response = $sdclient->post(config('agentconfig.tenant.tenant_url') . '/api/agent/ingest/papercut-data', [
                        'headers' => [],
                        'body' => json_encode($payload),
                    ]);

                    $status = json_decode($response->getBody(), false);

                    if($status->status != 'ok')
                    {
                        $this->info('Something went wrong while sending Papercut Data: '.$status->message);
                    }

                } catch (\Exception $e)
                {
                    \Log::error($e->getMessage());
                    \Log::error($e->getTraceAsString());
                    \Log::error('Could not send data to SchoolDesk Tenant');
                }

            }
        }

        return true;
    }
}
