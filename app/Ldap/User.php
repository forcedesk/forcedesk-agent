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

namespace App\Ldap;

use App\Ldap\Scopes\ImportFilter;
use Illuminate\Contracts\Auth\Authenticatable;
use LdapRecord\Models\Concerns\CanAuthenticate;
use LdapRecord\Models\Model;

class User extends Model implements Authenticatable
{
    use CanAuthenticate;

    public static $objectClasses = ['...'];

    protected $guidKey = 'uuid';

    protected static function boot()
    {
        parent::boot();

        static::addGlobalScope(new ImportFilter);
    }
}
