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

namespace App\Ldap\Rules;

use LdapRecord\Laravel\Auth\Rule;
use LdapRecord\Models\ActiveDirectory\Group;
use LdapRecord\Models\Model as LdapRecord;
use Illuminate\Database\Eloquent\Model as Eloquent;

class StudentGroupACL implements Rule
{
    /**
     * Check if the rule passes validation.
     *
     * @param LdapRecord $user
     * @param Eloquent|null $model
     * @return bool
     */
    public function passes(LdapRecord $user, Eloquent $model = null): bool
    {
        $studentgroup = Group::find(\App\Models\Settings::get_value('ldap_student_scope'));

        return $user->groups()->recursive()->exists($studentgroup);
    }
}
