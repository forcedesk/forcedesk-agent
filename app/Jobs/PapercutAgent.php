<?php

namespace App\Jobs;

use App\Models\Students;
use App\Models\User;
use GuzzleHttp\Client;
use Illuminate\Bus\Queueable;
use Illuminate\Contracts\Queue\ShouldQueue;
use Illuminate\Foundation\Bus\Dispatchable;
use Illuminate\Queue\InteractsWithQueue;
use Illuminate\Queue\SerializesModels;

class PapercutAgent implements ShouldQueue
{
    use Dispatchable, InteractsWithQueue, Queueable, SerializesModels;

    /**
     * Create a new job instance.
     */
    public function __construct()
    {
        //
    }

    /**
     * Execute the job.
     */
    public function handle(): void
    {
        $sdclient = new Client(['verify' => false, 'headers' => array(
            'Authorization' => 'Bearer ' . config('agentconfig.tenant.tenant_api_key'),
            'Content-Type' => 'application/json',
            'x-schooldesk-agent' => config('agentconfig.tenant.tenant_uuid'),
            'x-schooldesk-agentversion' => config('app.agent_version'),
        )]);

        $api_key = config('agentconfig.papercut.api_key');
        $api_url = config('agentconfig.papercut.api_url');

        $students = Students::whereNotNull('username')->orderby('username','asc')->get();
        $staff = User::whereNotNull('staff_code')->orderby('staff_code','asc')->get();

        if (!$students && !$staff) {
            $this->fail('No staff or students found to process.');
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
                    $payload['staff'][] = $data;
                }

            } catch (\Exception $e)
            {
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
                    $payload['students'][] = $data;
                }

            } catch (\Exception $e)
            {
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
                $this->fail('Something went wrong while sending Papercut Data: '.$status->message);
            }

        } catch (\Exception $e)
        {
            \Log::error('Could not send data to SchoolDesk Tenant');
        }

    }
}
