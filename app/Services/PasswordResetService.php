<?php

/****************************************************************************
 * SchoolDesk - The School Helpdesk System
 *
 * Copyright © 2019 - Excelion/Samuel Brereton. All Rights Reserved.
 *
 * This file or any other component of SchoolDesk cannot be copied, altered
 * and/or distributed without the express permission of SamueL Brereton.
 *
 * Your use of this software is governed by the SchoolDesk EULA. No warranty
 * is expressed or implied except otherwise laid out in your Support Agreement.
 *
 ***************************************************************************/

namespace App\Services;

use App\Http\Controllers\Controller;
use App\Models\EdupassAccounts;
use App\Models\Students;
use Illuminate\Support\Facades\Http;
use LdapRecord\Models\ActiveDirectory\Group as StudentGroup;
use LdapRecord\Models\ActiveDirectory\User as StudentUser;

class PasswordResetService extends Controller
{

    private function rp1()
    {
        $phrases = [
            'smiling',
            'guilty',
            'federal',
            'minor',
            'heavenly',
            'rare',
            'nice',
            'stale',
            'friendly',
            'unusual',
            'ritzy',
            'flimsy',
            'nippy',
            'sable',
            'daisy',
            'four',
            'afraid',
            'fluffy',
            'giant',
            'somber',
            'drunk',
            'erratic',
            'handsome',
            'workable',
            'worried',
            'unequal',
            'foolish',
            'tall',
            'learn',
            'united',
            'wakeful',
            'damaged',
            'panicky',
            'helpless',
            'familiar',
            'furtive',
            'sad',
            'tiny',
            'nature',
            'fragile',
            'conscious',
            'tiresome',
            'unused',
            'weakly',
            'deadly',
            'elated',
            'tanned',
            'existing',
            'fearless',
            'friendly',
            'unwieldy',
            'average',
            'keen',
            'thankful',
            'parallel',
            'spiteful',
            'abstracted',
            'hellish',
            'unable',
            'vigorous',
            'womanly',
            'cheaply',
            'silly',
            'afraid',
            'orange',
            'impartial',
            'basic',
            'majestic',
            'birdcage',
            'careless',
            'adamant',
            'skillful',
            'sticky',
            'melodic',
            'bright',
            'even',
            'married',
            'energetic',
            'wealthy',
            'truthful',
            'victorious',
            'oceanic',
            'responsible',
            'tasteful',
            'froggy'];

        return $phrases[array_rand($phrases)];
    }


    public function handlePasswordReset($kioskid, $username)
    {

        $dinopassgen = Http::get('https://dinopass.com/password/simple');
        $genpassword = ucfirst($dinopassgen);

        // We need to find the user based on the username provided instead of LDAP DN.
        $studentuser = StudentUser::where('samaccountname', $username)
            ->first();

        // Check if there is an Edupass Account.
        $edupassacct = EdupassAccounts::where('login', $username)->first();

        $group = StudentGroup::find(agent_config('ldap.student_scope'));

        if ($studentuser && $studentuser->groups()->exists($group)) {

            $studentuser->unicodepwd = $genpassword;

            try {
                $studentuser->save();

                return [
                    'action' => 'authorized',
                    'password' => $genpassword
                ];

            } catch (\LdapRecord\Exceptions\InsufficientAccessException $ex) {
                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'declined',
                    'message' => 'API Error. Contact Vendor.',
                ];

            } catch (\LdapRecord\Exceptions\ConstraintException $ex) {
                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'declined',
                    'message' => 'LDAP Error. Contact Vendor.',
                ];
            } catch (\LdapRecord\LdapRecordException $ex) {

                $error = $ex->getDetailedError();

                \Log::info($error->getErrorCode());
                \Log::info($error->getErrorMessage());
                \Log::info($error->getDiagnosticMessage());

                return [
                    'action' => 'declined',
                    'message' => 'LDAP Error. Contact Vendor.',
                ];
            }
        } elseif ($edupassacct) {

            if (empty(agent_config('emc.emc_username')) || empty(agent_config('emc.emc_password')) || empty(agent_config('emc.emc_school_code')) || empty(agent_config('emc.emc_url'))) {

                return [
                    'action' => 'declined',
                    'message' => 'Edustar API Error. Contact Vendor.',
                ];

            }

            if ($edupassacct->updated_at->diffInSeconds(\Carbon\Carbon::now()) <= 1800) {

                return [
                    'action' => 'declined',
                    'message' => 'You have changed your password recently.',
                ];

            }

            try {

                $emcpassword = ucwords(self::rp1()).'.'.rand(1000, 9999);

                $response = Http::withBasicAuth(agent_config('emc.emc_username'), agent_config('emc.emc_password'))
                    ->retry(5, 100)
                    ->post(agent_config('emc.emc_url'), [
                        'schoolId' => agent_config('emc.emc_school_code'),
                        'dn' => $edupassacct->ldap_dn,
                        'newPass' => $emcpassword,
                    ]);

                $edupassacct->password = $emcpassword;
                $edupassacct->save();

                return [
                    'action' => 'authorized',
                    'password' => $emcpassword,
                ];


            } catch (\Exception $e) {

                \Log::info($e->getMessage());

                return [
                    'action' => 'declined',
                    'payload' => 'Edustar API Error. Contact Vendor',
                ];

            }

        } else {
            return [
                'action' => 'declined',
                'payload' => 'Account does not exist. Check your code and try again.',
            ];
        }

    }
}
